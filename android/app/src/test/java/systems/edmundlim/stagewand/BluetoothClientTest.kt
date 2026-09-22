package systems.edmundlim.stagewand

import android.Manifest
import android.bluetooth.BluetoothAdapter
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothGattService
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.BluetoothStatusCodes
import android.bluetooth.le.BluetoothLeScanner
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.content.pm.PackageManager
import android.os.Looper
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.mockito.ArgumentMatchers.*
import org.mockito.AdditionalMatchers.aryEq
import org.mockito.Mockito.*
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.LooperMode
import systems.edmundlim.stagewand.protocol.BluetoothProtocol
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.KeyName
import java.time.Duration
import java.util.UUID

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [33], manifest = Config.NONE)
@LooperMode(LooperMode.Mode.PAUSED)
class BluetoothClientTest {
    private val context = mock(Context::class.java)
    private val manager = mock(BluetoothManager::class.java)
    private val adapter = mock(BluetoothAdapter::class.java)
    private val scanner = mock(BluetoothLeScanner::class.java)
    private val device = mock(BluetoothDevice::class.java)
    private val gatt = mock(BluetoothGatt::class.java)
    private val scanCallbacks = mutableListOf<ScanCallback>()
    private lateinit var callback: BluetoothGattCallback
    private lateinit var client: BluetoothClient
    private lateinit var command: BluetoothGattCharacteristic
    private lateinit var reply: BluetoothGattCharacteristic
    private lateinit var cccd: BluetoothGattDescriptor
    private val failures = mutableListOf<BluetoothClient.Failure>()
    private val received = mutableListOf<ByteArray>()
    private var readyCount = 0

    @Before
    fun setUp() {
        `when`(context.applicationContext).thenReturn(context)
        `when`(context.getSystemService(Context.BLUETOOTH_SERVICE)).thenReturn(manager)
        `when`(manager.adapter).thenReturn(adapter)
        `when`(adapter.isEnabled).thenReturn(true)
        `when`(adapter.bluetoothLeScanner).thenReturn(scanner)
        doAnswer { scanCallbacks.add(it.getArgument(2)); null }.`when`(scanner)
            .startScan(anyList(), any(ScanSettings::class.java), any(ScanCallback::class.java))
        doAnswer { callback = it.getArgument(2); gatt }.`when`(device)
            .connectGatt(eq(context), eq(false), any(BluetoothGattCallback::class.java), eq(BluetoothDevice.TRANSPORT_LE))
        `when`(gatt.requestMtu(517)).thenReturn(true)
        `when`(gatt.discoverServices()).thenReturn(true)
        `when`(gatt.setCharacteristicNotification(any(BluetoothGattCharacteristic::class.java), eq(true))).thenReturn(true)
        @Suppress("DEPRECATION")
        `when`(gatt.writeCharacteristic(any(BluetoothGattCharacteristic::class.java))).thenReturn(true)
        @Suppress("DEPRECATION")
        `when`(gatt.writeDescriptor(any(BluetoothGattDescriptor::class.java))).thenReturn(true)
        services()
        client = BluetoothClient(context)
    }

    @Test
    fun missingPermissionFailsBeforeTouchingTheRadio() {
        `when`(context.checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT)).thenReturn(PackageManager.PERMISSION_DENIED)
        connect()
        assertEquals(listOf(BluetoothClient.Failure.PermissionDenied), failures)
        verifyNoInteractions(adapter)
    }

    @Test
    @Config(sdk = [28])
    fun legacyAndroidRequiresLocationAndUsesTheLegacyGattWriteApi() {
        `when`(context.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION)).thenReturn(PackageManager.PERMISSION_DENIED)
        connect()
        assertEquals(listOf(BluetoothClient.Failure.PermissionDenied), failures)
        `when`(context.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION)).thenReturn(PackageManager.PERMISSION_GRANTED)
        makeReady()
        assertTrue(client.send(Command.Auth("1234")))
        @Suppress("DEPRECATION")
        verify(gatt).writeCharacteristic(command)
    }

    @Test
    fun rejectedMtuCannotSilentlyTruncateAuthentication() {
        connectGatt()
        callback.onMtuChanged(gatt, 23, BluetoothGatt.GATT_SUCCESS)
        drain()
        assertEquals(listOf(BluetoothClient.Failure.Incompatible), failures)
        assertEquals(0, readyCount)
        verify(gatt, never()).discoverServices()
        verify(gatt).close()
    }

    @Test
    fun mtuFailureRejectsDefaultCapacity() {
        connectGatt()
        callback.onMtuChanged(gatt, 517, BluetoothGatt.GATT_FAILURE)
        drain()
        assertEquals(listOf(BluetoothClient.Failure.Incompatible), failures)
    }

    @Test
    fun missingNotificationDescriptorCannotAuthenticate() {
        services(withDescriptor = false)
        discover()
        assertEquals(listOf(BluetoothClient.Failure.Incompatible), failures)
        assertEquals(0, readyCount)
    }

    @Test
    fun readyWaitsForSuccessfulSubscriptionToTheReply() {
        discover()
        assertEquals(0, readyCount)
        val unrelated = BluetoothGattDescriptor(UUID.randomUUID(), BluetoothGattDescriptor.PERMISSION_WRITE)
        reply.addDescriptor(unrelated)
        callback.onDescriptorWrite(gatt, unrelated, BluetoothGatt.GATT_SUCCESS)
        drain()
        assertEquals(0, readyCount)
        callback.onDescriptorWrite(gatt, cccd, BluetoothGatt.GATT_SUCCESS)
        drain()
        assertEquals(1, readyCount)
    }

    @Test
    fun fallsBackToAcknowledgedWritesAndIndications() {
        services(BluetoothGattCharacteristic.PROPERTY_WRITE, BluetoothGattCharacteristic.PROPERTY_INDICATE)
        makeReady()
        verify(gatt).writeDescriptor(eq(cccd), aryEq(BluetoothGattDescriptor.ENABLE_INDICATION_VALUE))
        assertTrue(client.send(Command.Auth("1234")))
        verify(gatt).writeCharacteristic(eq(command), any(ByteArray::class.java), eq(BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT))
    }

    @Test
    fun stalledWriteFailsAndClosesInsteadOfBlockingTheQueueForever() {
        makeReady()
        assertTrue(client.send(Command.Auth("1234")))
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(5))
        assertEquals(listOf(BluetoothClient.Failure.TimedOut), failures)
        verify(gatt).close()
    }

    @Test
    fun busyWritesRetryInOrderAndComplete() {
        `when`(gatt.writeCharacteristic(any(BluetoothGattCharacteristic::class.java), any(ByteArray::class.java), anyInt()))
            .thenReturn(BluetoothStatusCodes.ERROR_GATT_WRITE_REQUEST_BUSY, BluetoothStatusCodes.SUCCESS)
        makeReady()
        assertTrue(client.send(Command.Auth("1234")))
        assertTrue(client.send(Command.Key(KeyName.Right)))
        callback.onCharacteristicWrite(gatt, command, BluetoothGatt.GATT_SUCCESS)
        drain()
        callback.onCharacteristicWrite(gatt, command, BluetoothGatt.GATT_SUCCESS)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(6))
        assertTrue(failures.isEmpty())
        verify(gatt, times(3)).writeCharacteristic(eq(command), any(ByteArray::class.java), anyInt())
    }

    @Test
    fun staleScanResultsCannotConnectAfterCancelAndRestart() {
        connect()
        val old = scanCallbacks.single()
        client.cancel()
        connect()
        old.onScanResult(0, result())
        drain()
        verify(device, never()).connectGatt(any(Context::class.java), anyBoolean(), any(BluetoothGattCallback::class.java), anyInt())
        scanCallbacks.last().onScanResult(0, result())
        drain()
        verify(device).connectGatt(eq(context), eq(false), any(BluetoothGattCallback::class.java), eq(BluetoothDevice.TRANSPORT_LE))
    }

    @Test
    fun copiesNotificationsBeforePostingAndIgnoresOtherCharacteristics() {
        makeReady()
        val value = "{\"t\":\"status\"}".toByteArray()
        val expected = value.copyOf()
        callback.onCharacteristicChanged(gatt, command, value)
        callback.onCharacteristicChanged(gatt, reply, value)
        value.fill(0)
        drain()
        assertEquals(1, received.size)
        assertArrayEquals(expected, received.single())
    }

    @Test
    fun scanTimeoutAndCancellationReleaseResources() {
        connect()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(15))
        assertEquals(listOf(BluetoothClient.Failure.NotFound), failures)
        verify(scanner).stopScan(scanCallbacks.single())
        connect()
        client.cancel()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(30))
        assertEquals(1, failures.size)
    }

    @Test
    fun unexpectedMtuCallbackDoesNotRestartServiceDiscovery() {
        makeReady()
        callback.onMtuChanged(gatt, 517, BluetoothGatt.GATT_SUCCESS)
        drain()
        verify(gatt, times(1)).discoverServices()
        assertEquals(1, readyCount)
    }

    @Test
    fun serviceDatabaseChangesInvalidateCachedCharacteristics() {
        makeReady()
        callback.onServiceChanged(gatt)
        drain()
        assertEquals(listOf(BluetoothClient.Failure.Disconnected), failures)
        assertFalse(client.send(Command.Key(KeyName.Right)))
        verify(gatt).close()
    }

    @Test
    fun initialDefaultMtuDoesNotAbortBeforeNegotiation() {
        connect()
        scanCallbacks.last().onScanResult(0, result())
        drain()
        callback.onMtuChanged(gatt, 23, BluetoothGatt.GATT_SUCCESS)
        drain()
        assertTrue(failures.isEmpty())
        callback.onConnectionStateChange(gatt, BluetoothGatt.GATT_SUCCESS, BluetoothProfile.STATE_CONNECTED)
        drain()
        verify(gatt).requestMtu(517)
        callback.onMtuChanged(gatt, 185, BluetoothGatt.GATT_SUCCESS)
        drain()
        verify(gatt).discoverServices()
        assertTrue(failures.isEmpty())
    }

    private fun services(
        commandProperties: Int = BluetoothGattCharacteristic.PROPERTY_WRITE_NO_RESPONSE,
        replyProperties: Int = BluetoothGattCharacteristic.PROPERTY_NOTIFY,
        withDescriptor: Boolean = true
    ) {
        val service = BluetoothGattService(UUID.fromString(BluetoothProtocol.SERVICE), BluetoothGattService.SERVICE_TYPE_PRIMARY)
        command = BluetoothGattCharacteristic(UUID.fromString(BluetoothProtocol.COMMAND), commandProperties, BluetoothGattCharacteristic.PERMISSION_WRITE)
        reply = BluetoothGattCharacteristic(UUID.fromString(BluetoothProtocol.REPLY), replyProperties, BluetoothGattCharacteristic.PERMISSION_READ)
        cccd = BluetoothGattDescriptor(UUID.fromString("00002902-0000-1000-8000-00805f9b34fb"), BluetoothGattDescriptor.PERMISSION_WRITE)
        if (withDescriptor) reply.addDescriptor(cccd)
        service.addCharacteristic(command)
        service.addCharacteristic(reply)
        `when`(gatt.getService(service.uuid)).thenReturn(service)
    }

    private fun result(): ScanResult = mock(ScanResult::class.java).also {
        `when`(it.device).thenReturn(device)
    }

    private fun connect() = client.connect({ readyCount++ }, { received.add(it) }, { failures.add(it) })

    private fun connectGatt() {
        connect()
        scanCallbacks.last().onScanResult(0, result())
        drain()
        callback.onConnectionStateChange(gatt, BluetoothGatt.GATT_SUCCESS, BluetoothProfile.STATE_CONNECTED)
        drain()
    }

    private fun discover() {
        connectGatt()
        callback.onMtuChanged(gatt, 185, BluetoothGatt.GATT_SUCCESS)
        drain()
        callback.onServicesDiscovered(gatt, BluetoothGatt.GATT_SUCCESS)
        drain()
    }

    private fun makeReady() {
        discover()
        callback.onDescriptorWrite(gatt, cccd, BluetoothGatt.GATT_SUCCESS)
        drain()
    }

    private fun drain() = shadowOf(Looper.getMainLooper()).idle()
}
