package systems.edmundlim.stagewand

import android.annotation.SuppressLint
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelUuid
import systems.edmundlim.stagewand.protocol.BluetoothProtocol
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.CommandWriter
import java.util.UUID

class BluetoothClient(context: Context) {
    var onName: ((String) -> Unit)? = null

    private val app = context.applicationContext
    private val main = Handler(Looper.getMainLooper())
    private val serviceUUID = UUID.fromString(BluetoothProtocol.SERVICE)
    private val commandUUID = UUID.fromString(BluetoothProtocol.COMMAND)
    private val replyUUID = UUID.fromString(BluetoothProtocol.REPLY)
    private val cccdUUID = UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")

    private var wanted = false
    private var generation = 0
    private var scannerStarted = false
    private var gatt: BluetoothGatt? = null
    private var command: BluetoothGattCharacteristic? = null
    private var ready = false
    private val outbound = CommandWriter { command ->
        val connection = gatt
        val characteristic = this.command
        ready && connection != null && characteristic != null &&
            writeCharacteristic(connection, characteristic, command.encode().toByteArray(Charsets.UTF_8))
    }
    private val retryWrite = Runnable { flush() }
    private var onReady: (() -> Unit)? = null
    private var onData: ((ByteArray) -> Unit)? = null
    private var onFail: (() -> Unit)? = null

    private val adapter: BluetoothAdapter?
        get() = (app.getSystemService(Context.BLUETOOTH_SERVICE) as? BluetoothManager)?.adapter

    fun connect(onReady: () -> Unit, onData: (ByteArray) -> Unit, onFail: () -> Unit) {
        cancel()
        generation += 1
        this.onReady = onReady
        this.onData = onData
        this.onFail = onFail
        wanted = true
        ready = false
        outbound.clear()
        main.removeCallbacks(retryWrite)
        val radio = adapter
        if (radio == null || !radio.isEnabled) {
            fail()
            return
        }
        scan()
    }

    fun send(command: Command): Boolean {
        if (!ready) return false
        if (!outbound.enqueue(command)) return false
        flush()
        return true
    }

    fun cancel() {
        wanted = false
        ready = false
        generation += 1
        outbound.clear()
        main.removeCallbacks(retryWrite)
        stopScan()
        command = null
        gatt?.disconnect()
        gatt?.close()
        gatt = null
        onReady = null
        onData = null
        onFail = null
    }

    @SuppressLint("MissingPermission")
    private fun scan() {
        val radio = adapter ?: return fail()
        val scanner = radio.bluetoothLeScanner ?: return fail()
        if (scannerStarted) return
        scannerStarted = true
        val filter = ScanFilter.Builder().setServiceUuid(ParcelUuid(serviceUUID)).build()
        val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
        try {
            scanner.startScan(listOf(filter), settings, scanCallback)
        } catch (_: SecurityException) {
            scannerStarted = false
            fail()
        }
    }

    private val scanCallback = object : ScanCallback() {
        override fun onScanResult(callbackType: Int, result: ScanResult) {
            main.post {
                if (!wanted || gatt != null) return@post
                stopScan()
                val device = result.device
                onName?.invoke(deviceName(device) ?: "host")
                connectGatt(device)
            }
        }

        override fun onScanFailed(errorCode: Int) {
            main.post { fail() }
        }
    }

    @SuppressLint("MissingPermission")
    private fun connectGatt(device: BluetoothDevice) {
        val id = generation
        gatt = if (Build.VERSION.SDK_INT >= 23) {
            device.connectGatt(app, false, gattCallback, BluetoothDevice.TRANSPORT_LE)
        } else {
            device.connectGatt(app, false, gattCallback)
        }
        main.postDelayed({
            if (generation == id && wanted && !ready) fail()
        }, 10_000)
    }

    private val gattCallback = object : BluetoothGattCallback() {
        override fun onConnectionStateChange(gatt: BluetoothGatt, status: Int, newState: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                if (newState == BluetoothProfile.STATE_CONNECTED) {
                    if (!gatt.requestMtu(517)) {
                        gatt.discoverServices()
                    }
                    return@post
                }
                if (newState == BluetoothProfile.STATE_DISCONNECTED) fail()
            }
        }

        override fun onMtuChanged(gatt: BluetoothGatt, mtu: Int, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                gatt.discoverServices()
            }
        }

        override fun onServicesDiscovered(gatt: BluetoothGatt, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                val service = gatt.getService(serviceUUID)
                val commandChar = service?.getCharacteristic(commandUUID)
                val replyChar = service?.getCharacteristic(replyUUID)
                if (commandChar == null || replyChar == null) {
                    fail()
                    return@post
                }
                command = commandChar
                if (!gatt.setCharacteristicNotification(replyChar, true)) {
                    fail()
                    return@post
                }
                val cccd = replyChar.getDescriptor(cccdUUID)
                if (cccd == null) {
                    markReady()
                    return@post
                }
                if (!writeDescriptor(gatt, cccd, BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE)) {
                    fail()
                }
            }
        }

        override fun onDescriptorWrite(gatt: BluetoothGatt, descriptor: BluetoothGattDescriptor, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                markReady()
            }
        }

        @Suppress("DEPRECATION")
        override fun onCharacteristicChanged(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic) {
            val value = characteristic.value ?: return
            if (value.isEmpty()) return
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                onData?.invoke(value.copyOf())
            }
        }

        override fun onCharacteristicChanged(
            gatt: BluetoothGatt,
            characteristic: BluetoothGattCharacteristic,
            value: ByteArray
        ) {
            if (value.isEmpty()) return
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                onData?.invoke(value.copyOf())
            }
        }

        override fun onCharacteristicWrite(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic, status: Int) {
            main.post {
                if (gatt != this@BluetoothClient.gatt) return@post
                if (status != BluetoothGatt.GATT_SUCCESS) {
                    fail()
                    return@post
                }
                outbound.completed()
                flush()
            }
        }
    }

    private fun markReady() {
        if (ready) return
        ready = true
        onReady?.invoke()
        flush()
    }

    private fun flush() {
        if (!ready) return
        main.removeCallbacks(retryWrite)
        outbound.flush()
        if (outbound.needsRetry) main.postDelayed(retryWrite, 20)
    }

    @SuppressLint("MissingPermission")
    private fun writeCharacteristic(
        gatt: BluetoothGatt,
        characteristic: BluetoothGattCharacteristic,
        data: ByteArray
    ): Boolean {
        return if (Build.VERSION.SDK_INT >= 33) {
            gatt.writeCharacteristic(characteristic, data, BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE) == BluetoothGatt.GATT_SUCCESS
        } else {
            @Suppress("DEPRECATION")
            characteristic.writeType = BluetoothGattCharacteristic.WRITE_TYPE_NO_RESPONSE
            @Suppress("DEPRECATION")
            characteristic.value = data
            @Suppress("DEPRECATION")
            gatt.writeCharacteristic(characteristic)
        }
    }

    @SuppressLint("MissingPermission")
    private fun writeDescriptor(
        gatt: BluetoothGatt,
        descriptor: BluetoothGattDescriptor,
        value: ByteArray
    ): Boolean {
        return if (Build.VERSION.SDK_INT >= 33) {
            gatt.writeDescriptor(descriptor, value) == BluetoothGatt.GATT_SUCCESS
        } else {
            @Suppress("DEPRECATION")
            descriptor.value = value
            @Suppress("DEPRECATION")
            gatt.writeDescriptor(descriptor)
        }
    }

    @SuppressLint("MissingPermission")
    private fun stopScan() {
        if (!scannerStarted) return
        scannerStarted = false
        try {
            adapter?.bluetoothLeScanner?.stopScan(scanCallback)
        } catch (_: RuntimeException) {
        }
    }

    @SuppressLint("MissingPermission")
    private fun fail() {
        if (!wanted) return
        wanted = false
        ready = false
        outbound.clear()
        main.removeCallbacks(retryWrite)
        stopScan()
        command = null
        generation += 1
        gatt?.disconnect()
        gatt?.close()
        gatt = null
        onFail?.invoke()
    }

    @SuppressLint("MissingPermission")
    private fun deviceName(device: BluetoothDevice): String? {
        return try {
            device.name
        } catch (_: SecurityException) {
            null
        }
    }

}
