package systems.edmundlim.stagewand

import android.os.Looper
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.mockito.kotlin.any
import org.mockito.kotlin.anyOrNull
import org.mockito.Mockito.*
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.LooperMode
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.KeyName
import java.time.Duration

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [33], manifest = Config.NONE)
@LooperMode(LooperMode.Mode.PAUSED)
class LinkTest {
    private data class Attempt(val ready: () -> Unit, val data: (ByteArray) -> Unit, val fail: (BluetoothClient.Failure) -> Unit)
    private val bluetooth = mock(BluetoothClient::class.java)
    private val discovery = mock(Discovery::class.java)
    private val attempts = mutableListOf<Attempt>()
    private lateinit var link: Link
    private var onResults: (() -> Unit)? = null
    private var onDenied: (() -> Unit)? = null

    @Before
    fun setUp() {
        doAnswer {
            attempts.add(Attempt(it.getArgument(0), it.getArgument(1), it.getArgument(2)))
            null
        }.`when`(bluetooth).connect(any(), any(), any())
        `when`(bluetooth.send(any())).thenReturn(true)
        doAnswer { onResults = it.getArgument(0); null }.`when`(discovery).onResults = anyOrNull()
        doAnswer { onDenied = it.getArgument(0); null }.`when`(discovery).onDenied = anyOrNull()
        `when`(discovery.results).thenReturn(emptyList())
        link = Link(discovery, bluetooth)
    }

    @After
    fun tearDown() {
        if (::link.isInitialized) link.close()
    }

    @Test
    fun settingsLoadDoesNotConnectBeforePermissionsAreResolved() {
        link.update(Settings(pairingCode = "1234"))
        assertTrue(attempts.isEmpty())
        link.start()
        assertEquals(1, attempts.size)
    }

    @Test
    fun partialAndNonAsciiPairingCodesNeverReachTheHost() {
        link.start()
        for (code in listOf("", "1", "123", "١٢٣٤", "12345")) {
            link.update(Settings(pairingCode = code))
            assertEquals(Link.State.EnterCode, link.state)
        }
        assertTrue(attempts.isEmpty())
        link.update(Settings(pairingCode = "0123"))
        assertEquals(1, attempts.size)
    }

    @Test
    fun authenticationPrecedesCommandsAndDisconnectedInputIsNotReplayed() {
        start()
        link.send(Command.Key(KeyName.Right))
        attempts.single().ready()
        drain()
        verify(bluetooth).send(Command.Auth("1234"))
        verify(bluetooth, never()).send(Command.Key(KeyName.Right))
        attempts.single().data("{\"t\":\"status\"}".toByteArray())
        drain()
        assertEquals(Link.State.Authed, link.state)
        verify(bluetooth, never()).send(Command.Key(KeyName.Right))
        link.send(Command.Key(KeyName.Right))
        verify(bluetooth).send(Command.Key(KeyName.Right))
    }

    @Test
    fun incompatibleBluetoothConnectionHasActionableErrorWithoutRetryLoop() {
        start()
        attempts.single().fail(BluetoothClient.Failure.Incompatible)
        drain()
        assertEquals(Link.State.Disconnected, link.state)
        assertTrue(link.failureMessage!!.contains("Wi-Fi"))
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(60))
        assertEquals(1, attempts.size)
    }

    @Test
    fun reconnectBackoffGrowsAndOldCallbacksCannotAuthenticateTheNewConnection() {
        start()
        val first = attempts.single()
        first.fail(BluetoothClient.Failure.Disconnected)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(1))
        assertEquals(2, attempts.size)
        first.data("{\"t\":\"status\"}".toByteArray())
        drain()
        assertEquals(Link.State.Connecting, link.state)
        attempts.last().fail(BluetoothClient.Failure.Disconnected)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(1))
        assertEquals(2, attempts.size)
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(1))
        assertEquals(3, attempts.size)
    }

    @Test
    fun wrongCodeStopsReconnectUntilUserChangesSettings() {
        start()
        attempts.single().data("{\"t\":\"bye\",\"reason\":\"badauth\"}".toByteArray())
        drain()
        assertEquals(Link.State.EnterCode, link.state)
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(60))
        assertEquals(1, attempts.size)
        link.update(Settings(pairingCode = "5678"))
        assertEquals(2, attempts.size)
    }

    @Test
    fun permissionFailureIsExplicitAndClosePreventsFurtherCallbacks() {
        start()
        val attempt = attempts.single()
        attempt.fail(BluetoothClient.Failure.PermissionDenied)
        drain()
        assertEquals(Link.State.BluetoothDenied, link.state)
        link.close()
        attempt.data("{\"t\":\"status\"}".toByteArray())
        drain()
        assertEquals(Link.State.BluetoothDenied, link.state)
    }

    @Test
    fun stoppingDiscoveryCannotReenterBluetoothConnection() {
        doAnswer { onResults?.invoke(); null }.`when`(discovery).stop()
        start()
        assertEquals(1, attempts.size)
    }

    @Test
    fun synchronousDiscoveryFailureDoesNotOverwriteDeniedState() {
        doAnswer { onDenied?.invoke(); null }.`when`(discovery).start()
        link.update(Settings(pairingCode = "1234", transport = "wifi"))
        link.start()
        assertEquals(Link.State.LocalNetworkDenied, link.state)
        doNothing().`when`(discovery).start()
        link.start()
        assertEquals(Link.State.Searching, link.state)
    }

    @Test
    fun malformedUtf8ReplyCannotBeReinterpretedAsAValidBye() {
        start()
        val attempt = attempts.single()
        attempt.data("{\"t\":\"status\"}".toByteArray())
        drain()
        assertEquals(Link.State.Authed, link.state)
        attempt.data("{\"t\":\"bye\",\"reason\":\"".toByteArray() + byteArrayOf(0xff.toByte()) + "\"}".toByteArray())
        drain()
        assertEquals(Link.State.Authed, link.state)
    }

    @Test
    fun synchronousSendFailureKeepsItsDiagnosticAndRetryPolicy() {
        start()
        doAnswer {
            attempts.single().fail(BluetoothClient.Failure.Incompatible)
            false
        }.`when`(bluetooth).send(any())
        attempts.single().ready()
        drain()
        assertEquals(BluetoothClient.Failure.Incompatible.message, link.failureMessage)
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(60))
        assertEquals(1, attempts.size)
    }

    @Test
    fun unsupportedDiscoveredIpv6ScopeCannotCrashTheApp() {
        `when`(discovery.results).thenReturn(listOf(HostService("host", "fe80::1%wlan0", 9000)))
        link.update(Settings(pairingCode = "1234", transport = "wifi"))
        link.start()
        assertEquals(Link.State.Disconnected, link.state)
        assertTrue(link.failureMessage!!.contains("IPv4"))
    }

    private fun start() {
        link.update(Settings(pairingCode = "1234"))
        link.start()
    }

    private fun drain() = shadowOf(Looper.getMainLooper()).idle()
}
