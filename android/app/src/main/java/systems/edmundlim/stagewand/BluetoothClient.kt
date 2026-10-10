package systems.edmundlim.stagewand

import android.Manifest
import android.annotation.SuppressLint
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.BluetoothStatusCodes
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelUuid
import systems.edmundlim.stagewand.protocol.BluetoothProtocol
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.CommandWriter
import java.util.UUID

@SuppressLint("MissingPermission")
class BluetoothClient(context: Context) {
    enum class Failure(val message: String, val retryable: Boolean = true) {
        PermissionDenied("Allow Bluetooth permission in Android settings, then reopen Stage Wand.", false),
        Unavailable("Turn on Bluetooth, or switch to Wi-Fi in Settings."),
        NotFound("No Bluetooth host found. Enable Bluetooth on the host and keep it nearby."),
        Incompatible("This host cannot carry Stage Wand Bluetooth messages. Update the host or use Wi-Fi.", false),
        Disconnected("Bluetooth disconnected. Reconnecting…"),
        TimedOut("Bluetooth stopped responding. Reconnecting…")
    }

    var onName: ((String) -> Unit)? = null

    private enum class Phase { Idle, Connecting, Negotiating, Discovering, Subscribing, Ready }

    private val app = context.applicationContext
    private val main = Handler(Looper.getMainLooper())
    private val serviceUUID = UUID.fromString(BluetoothProtocol.SERVICE)
    private val commandUUID = UUID.fromString(BluetoothProtocol.COMMAND)
    private val replyUUID = UUID.fromString(BluetoothProtocol.REPLY)
    private val cccdUUID = UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")

    private var wanted = false
    private var generation = 0
    private var scanner: BluetoothLeScanner? = null
    private var scanCallback: ScanCallback? = null
    private var gatt: BluetoothGatt? = null
    private var command: BluetoothGattCharacteristic? = null
    private var phase = Phase.Idle
    private var payloadCapacity = 20
    private var writeType = BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE
    private var writePending = false
    private var writeDeadlineArmed = false
    private val outbound = CommandWriter { value ->
        val connection = gatt
        val characteristic = command
        if (phase != Phase.Ready || connection == null || characteristic == null) {
            false
        } else {
            val data = value.encode().toByteArray(Charsets.UTF_8)
            if (data.size > payloadCapacity || data.size > BluetoothProtocol.MAX_FRAME_BYTES) {
                fail(Failure.Incompatible)
                false
            } else {
                writeCharacteristic(connection, characteristic, data).also { writePending = it }
            }
        }
    }
    private val retryWrite = Runnable { flush() }
    private val writeDeadline = Runnable {
        writeDeadlineArmed = false
        if (wanted && outbound.count > 0) fail(Failure.TimedOut)
    }
    private val setupDeadline = Runnable {
        if (wanted && phase != Phase.Ready) {
            fail(if (phase == Phase.Idle) Failure.NotFound else Failure.TimedOut)
        }
    }
    private var onReady: (() -> Unit)? = null
    private var onData: ((ByteArray) -> Unit)? = null
    private var onFail: ((Failure) -> Unit)? = null

    private val adapter: BluetoothAdapter?
        get() = (app.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter

    fun connect(onReady: () -> Unit, onData: (ByteArray) -> Unit, onFail: (Failure) -> Unit) {
        cancel()
        this.onReady = onReady
        this.onData = onData
        this.onFail = onFail
        wanted = true
        if (!hasPermission()) {
            fail(Failure.PermissionDenied)
            return
        }
        radioOperation {
            val radio = adapter
            if (radio == null || !radio.isEnabled) {
                fail(Failure.Unavailable)
            } else {
                main.postDelayed(setupDeadline, 15_000)
                scan(radio)
            }
        }
    }

    fun send(command: Command): Boolean {
        if (phase != Phase.Ready) return false
        val size = command.encode().toByteArray(Charsets.UTF_8).size
        if (size > payloadCapacity || size > BluetoothProtocol.MAX_FRAME_BYTES) {
            fail(Failure.Incompatible)
            return false
        }
        if (!outbound.enqueue(command)) {
            fail(Failure.TimedOut)
            return false
        }
        flush()
        return phase == Phase.Ready
    }

    fun cancel() {
        wanted = false
        generation += 1
        phase = Phase.Idle
        outbound.clear()
        writePending = false
        writeDeadlineArmed = false
        main.removeCallbacksAndMessages(null)
        stopScan()
        command = null
        payloadCapacity = 20
        val connection = gatt
        gatt = null
        try {
            connection?.disconnect()
        } catch (_: RuntimeException) {
            // Permission can be revoked while a connection is open.
        } finally {
            try {
                connection?.close()
            } catch (_: RuntimeException) {
            }
        }
        onReady = null
        onData = null
        onFail = null
    }

    private fun scan(radio: BluetoothAdapter) {
        val activeScanner = radio.bluetoothLeScanner ?: return fail(Failure.Unavailable)
        val id = generation
        val callback = object : ScanCallback() {
            override fun onScanResult(callbackType: Int, result: ScanResult) {
                main.post {
                    if (generation != id || !wanted || phase != Phase.Idle || gatt != null) return@post
                    stopScan()
                    onName?.invoke(deviceName(result.device) ?: "host")
                    connectGatt(result.device)
                }
            }

            override fun onScanFailed(errorCode: Int) {
                main.post { if (generation == id && wanted && phase == Phase.Idle) fail() }
            }
        }
        scanner = activeScanner
        scanCallback = callback
        val filter = ScanFilter.Builder().setServiceUuid(ParcelUuid(serviceUUID)).build()
        val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
        activeScanner.startScan(listOf(filter), settings, callback)
    }

    private fun connectGatt(device: BluetoothDevice) {
        phase = Phase.Connecting
        radioOperation {
            gatt = device.connectGatt(app, false, gattCallback, BluetoothDevice.TRANSPORT_LE)
            if (gatt == null) fail()
        }
    }

    private val gattCallback = object : BluetoothGattCallback() {
        override fun onConnectionStateChange(gatt: BluetoothGatt, status: Int, newState: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt || !wanted) return@post
                if (status != BluetoothGatt.GATT_SUCCESS || newState == BluetoothProfile.STATE_DISCONNECTED) {
                    fail()
                } else if (newState == BluetoothProfile.STATE_CONNECTED && phase == Phase.Connecting) {
                    radioOperation {
                        if (payloadCapacity >= BluetoothProtocol.MIN_COMMAND_BYTES) {
                            phase = Phase.Discovering
                            if (!gatt.discoverServices()) fail()
                        } else {
                            phase = Phase.Negotiating
                            if (!gatt.requestMtu(517)) fail(Failure.Incompatible)
                        }
                    }
                }
            }
        }

        override fun onServiceChanged(gatt: BluetoothGatt) {
            main.post { if (gatt == this@BluetoothClient.gatt && wanted) fail() }
        }

        override fun onMtuChanged(gatt: BluetoothGatt, mtu: Int, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt || !wanted) return@post
                if (status == BluetoothGatt.GATT_SUCCESS) payloadCapacity = (mtu - 3).coerceIn(20, 512)
                if (phase == Phase.Connecting) return@post
                if (payloadCapacity < BluetoothProtocol.MIN_COMMAND_BYTES) {
                    fail(Failure.Incompatible)
                } else if (phase == Phase.Negotiating) {
                    phase = Phase.Discovering
                    radioOperation { if (!gatt.discoverServices()) fail() }
                }
            }
        }

        override fun onServicesDiscovered(gatt: BluetoothGatt, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt || phase != Phase.Discovering) return@post
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                radioOperation {
                    val service = gatt.getService(serviceUUID)
                    val commandChar = service?.getCharacteristic(commandUUID)
                    val replyChar = service?.getCharacteristic(replyUUID)
                    if (commandChar == null || replyChar == null) {
                        fail(Failure.Incompatible)
                        return@radioOperation
                    }
                    writeType = when {
                        commandChar.properties and BluetoothGattCharacteristic.PROPERTY_WRITE_NO_RESPONSE != 0 ->
                            BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE
                        commandChar.properties and BluetoothGattCharacteristic.PROPERTY_WRITE != 0 ->
                            BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT
                        else -> {
                            fail(Failure.Incompatible)
                            return@radioOperation
                        }
                    }
                    val notifyValue = when {
                        replyChar.properties and BluetoothGattCharacteristic.PROPERTY_NOTIFY != 0 ->
                            BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE
                        replyChar.properties and BluetoothGattCharacteristic.PROPERTY_INDICATE != 0 ->
                            BluetoothGattDescriptor.ENABLE_INDICATION_VALUE
                        else -> {
                            fail(Failure.Incompatible)
                            return@radioOperation
                        }
                    }
                    val cccd = replyChar.getDescriptor(cccdUUID)
                    if (cccd == null) {
                        fail(Failure.Incompatible)
                        return@radioOperation
                    }
                    command = commandChar
                    phase = Phase.Subscribing
                    if (!gatt.setCharacteristicNotification(replyChar, true) || !writeDescriptor(gatt, cccd, notifyValue)) fail()
                }
            }
        }

        override fun onDescriptorWrite(gatt: BluetoothGatt, descriptor: BluetoothGattDescriptor, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt || phase != Phase.Subscribing ||
                    descriptor.uuid != cccdUUID || descriptor.characteristic?.uuid != replyUUID) return@post
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                phase = Phase.Ready
                main.removeCallbacks(setupDeadline)
                onReady?.invoke()
                flush()
            }
        }

        @Suppress("DEPRECATION")
        override fun onCharacteristicChanged(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic) {
            characteristic.value?.let { receive(gatt, characteristic, it) }
        }

        override fun onCharacteristicChanged(
            gatt: BluetoothGatt,
            characteristic: BluetoothGattCharacteristic,
            value: ByteArray
        ) = receive(gatt, characteristic, value)

        override fun onCharacteristicWrite(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt || phase != Phase.Ready ||
                    characteristic.uuid != commandUUID || !writePending) return@post
                writePending = false
                main.removeCallbacks(writeDeadline)
                writeDeadlineArmed = false
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                outbound.completed()
                flush()
            }
        }
    }

    private fun receive(connection: BluetoothGatt, characteristic: BluetoothGattCharacteristic, value: ByteArray) {
        if (characteristic.uuid != replyUUID || value.isEmpty()) return
        val snapshot = value.copyOf()
        main.post {
            if (connection != gatt || phase != Phase.Ready) return@post
            if (snapshot.size > BluetoothProtocol.MAX_FRAME_BYTES) fail(Failure.Incompatible)
            else onData?.invoke(snapshot)
        }
    }

    private fun flush() {
        if (phase != Phase.Ready) return
        main.removeCallbacks(retryWrite)
        outbound.flush()
        if (phase != Phase.Ready) return
        if (outbound.count > 0 && !writeDeadlineArmed) {
            writeDeadlineArmed = true
            main.postDelayed(writeDeadline, 5_000)
        }
        if (outbound.needsRetry) main.postDelayed(retryWrite, 20)
    }

    private fun writeCharacteristic(
        gatt: BluetoothGatt,
        characteristic: BluetoothGattCharacteristic,
        data: ByteArray
    ): Boolean {
        var accepted = false
        radioOperation {
            accepted = if (Build.VERSION.SDK_INT >= 33) {
                val status = gatt.writeCharacteristic(characteristic, data, writeType)
                if (status != BluetoothStatusCodes.SUCCESS && status != BluetoothStatusCodes.ERROR_GATT_WRITE_REQUEST_BUSY) fail()
                status == BluetoothStatusCodes.SUCCESS
            } else {
                @Suppress("DEPRECATION")
                characteristic.writeType = writeType
                @Suppress("DEPRECATION")
                characteristic.value = data
                @Suppress("DEPRECATION")
                gatt.writeCharacteristic(characteristic)
            }
        }
        return accepted
    }

    private fun writeDescriptor(gatt: BluetoothGatt, descriptor: BluetoothGattDescriptor, value: ByteArray): Boolean {
        return if (Build.VERSION.SDK_INT >= 33) {
            gatt.writeDescriptor(descriptor, value) == BluetoothStatusCodes.SUCCESS
        } else {
            @Suppress("DEPRECATION")
            descriptor.value = value
            @Suppress("DEPRECATION")
            gatt.writeDescriptor(descriptor)
        }
    }

    private fun stopScan() {
        val activeScanner = scanner
        val callback = scanCallback
        scanner = null
        scanCallback = null
        if (activeScanner != null && callback != null) {
            try {
                activeScanner.stopScan(callback)
            } catch (_: RuntimeException) {
            }
        }
    }

    private fun fail(reason: Failure = Failure.Disconnected) {
        if (!wanted) return
        val callback = onFail
        cancel()
        callback?.invoke(reason)
    }

    private inline fun radioOperation(operation: () -> Unit) {
        try {
            operation()
        } catch (_: SecurityException) {
            fail(Failure.PermissionDenied)
        } catch (_: IllegalStateException) {
            fail(Failure.Unavailable)
        } catch (_: IllegalArgumentException) {
            fail(Failure.Incompatible)
        }
    }

    private fun hasPermission(): Boolean = if (Build.VERSION.SDK_INT >= 31) {
        app.checkSelfPermission(Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED &&
            app.checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED
    } else {
        app.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
    }

    private fun deviceName(device: BluetoothDevice): String? = try {
        device.name
    } catch (_: SecurityException) {
        null
    }
}
