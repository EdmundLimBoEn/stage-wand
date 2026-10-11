package systems.edmundlim.stagewand

import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.mockito.ArgumentMatchers.any
import org.mockito.ArgumentMatchers.anyInt
import org.mockito.ArgumentMatchers.anyString
import org.mockito.Mockito.doAnswer
import org.mockito.Mockito.mock
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows.shadowOf
import org.robolectric.annotation.Config
import org.robolectric.annotation.LooperMode
import java.net.InetAddress
import java.time.Duration

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [28], manifest = Config.NONE)
@LooperMode(LooperMode.Mode.PAUSED)
class DiscoveryTest {
    private val nsd = mock(NsdManager::class.java)
    private val listeners = mutableListOf<NsdManager.DiscoveryListener>()
    private val stoppedListeners = mutableListOf<NsdManager.DiscoveryListener>()
    private val resolutions = mutableListOf<Pair<NsdServiceInfo, NsdManager.ResolveListener>>()
    private lateinit var discovery: Discovery

    @Before
    fun setUp() {
        doAnswer {
            listeners.add(it.getArgument(2))
            null
        }.`when`(nsd).discoverServices(anyString(), anyInt(), any(NsdManager.DiscoveryListener::class.java))
        doAnswer {
            resolutions.add(it.getArgument<NsdServiceInfo>(0) to it.getArgument(1))
            null
        }.`when`(nsd).resolveService(any(NsdServiceInfo::class.java), any(NsdManager.ResolveListener::class.java))
        doAnswer {
            stoppedListeners.add(it.getArgument(0))
            null
        }.`when`(nsd).stopServiceDiscovery(any(NsdManager.DiscoveryListener::class.java))
        discovery = Discovery(nsd, Handler(Looper.getMainLooper()))
    }

    @Test
    fun resolvesMultipleHostsSerially() {
        discovery.start()
        listeners.single().onServiceFound(service("B"))
        listeners.single().onServiceFound(service("A"))
        drain()
        assertEquals(listOf("B"), resolutions.map { it.first.serviceName })

        resolve(0)
        assertEquals(listOf("B", "A"), resolutions.map { it.first.serviceName })
        resolve(1)
        assertEquals(listOf("A", "B"), discovery.results.map { it.name })
    }

    @Test
    fun lostHostCannotBeResurrectedByResolution() {
        discovery.start()
        val listener = listeners.single()
        listener.onServiceFound(service("gone"))
        listener.onServiceFound(service("remaining"))
        drain()
        listener.onServiceLost(service("gone"))
        drain()
        resolve(0)
        assertTrue(discovery.results.isEmpty())
        resolve(1)
        assertEquals(listOf("remaining"), discovery.results.map { it.name })
    }

    @Test
    fun restartIgnoresOldCallbacksAndWaitsForOutstandingResolution() {
        discovery.start()
        val oldListener = listeners.single()
        oldListener.onServiceFound(service("old"))
        drain()
        discovery.stop()
        discovery.start()
        val currentListener = listeners.last()
        currentListener.onServiceFound(service("current"))
        oldListener.onServiceFound(service("late"))
        oldListener.onStartDiscoveryFailed("_stagewand._tcp.", NsdManager.FAILURE_INTERNAL_ERROR)
        oldListener.onDiscoveryStopped("_stagewand._tcp.")
        drain()
        assertEquals(1, resolutions.size)
        resolve(0)
        assertTrue(discovery.results.isEmpty())
        assertEquals(listOf("old", "current"), resolutions.map { it.first.serviceName })
        resolve(1)
        oldListener.onServiceLost(service("current"))
        drain()
        assertEquals(listOf("current"), discovery.results.map { it.name })
    }

    @Test
    fun stopFailureRetriesOriginalListenerWithoutDisturbingNewSession() {
        discovery.start()
        val oldListener = listeners.single()
        discovery.stop()
        discovery.start()
        oldListener.onStopDiscoveryFailed("_stagewand._tcp.", NsdManager.FAILURE_INTERNAL_ERROR)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(250))
        assertEquals(listOf(oldListener, oldListener), stoppedListeners)
        oldListener.onDiscoveryStopped("_stagewand._tcp.")
        listeners.last().onServiceFound(service("current"))
        drain()
        resolve(0)
        assertEquals(listOf("current"), discovery.results.map { it.name })
    }

    @Test
    fun failedStartCanBeRetried() {
        var denied = 0
        discovery.onDenied = { denied++ }
        discovery.start()
        listeners.single().onStartDiscoveryFailed("_stagewand._tcp.", NsdManager.FAILURE_INTERNAL_ERROR)
        drain()
        assertEquals(1, denied)
        discovery.start()
        assertEquals(2, listeners.size)
        listeners.last().onServiceFound(service("retry"))
        drain()
        resolve(0)
        assertEquals(listOf("retry"), discovery.results.map { it.name })
    }

    @Test
    fun stopPublishesEmptyResults() {
        discovery.start()
        listeners.single().onServiceFound(service("host"))
        drain()
        resolve(0)
        var notified = false
        discovery.onResults = { notified = true }
        discovery.stop()
        assertTrue(notified)
        assertTrue(discovery.results.isEmpty())
    }

    @Test
    fun failedResolutionRetriesAreBoundedAndDoNotBlockOtherHosts() {
        discovery.start()
        listeners.single().onServiceFound(service("busy"))
        listeners.single().onServiceFound(service("ready"))
        drain()
        resolutions[0].second.onResolveFailed(null, NsdManager.FAILURE_ALREADY_ACTIVE)
        drain()
        assertEquals(listOf("busy", "ready"), resolutions.map { it.first.serviceName })
        resolve(1)
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(250))
        resolutions[2].second.onResolveFailed(null, NsdManager.FAILURE_INTERNAL_ERROR)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofMillis(500))
        resolutions[3].second.onResolveFailed(null, NsdManager.FAILURE_INTERNAL_ERROR)
        drain()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(10))
        assertEquals(4, resolutions.size)
        assertEquals(listOf("ready"), discovery.results.map { it.name })
    }

    @Test
    fun stoppingCancelsScheduledResolutionRetries() {
        discovery.start()
        listeners.single().onServiceFound(service("busy"))
        drain()
        resolutions.single().second.onResolveFailed(null, NsdManager.FAILURE_ALREADY_ACTIVE)
        drain()
        discovery.stop()
        shadowOf(Looper.getMainLooper()).idleFor(Duration.ofSeconds(10))
        assertEquals(1, resolutions.size)
        assertTrue(discovery.results.isEmpty())
    }

    @Test
    fun startingWhileActiveDoesNotRegisterAnotherListener() {
        discovery.start()
        discovery.start()
        assertEquals(1, listeners.size)
    }

    @Test
    fun lostAndRediscoveredNameOnlyPublishesLatestResolution() {
        discovery.start()
        val listener = listeners.single()
        listener.onServiceFound(service("host"))
        drain()
        listener.onServiceLost(service("host"))
        listener.onServiceFound(service("host"))
        drain()
        resolve(0)
        assertTrue(discovery.results.isEmpty())
        resolve(1)
        assertEquals(1, discovery.results.size)
    }

    @Test
    fun resolvedHostWithInvalidPortIsNotPublished() {
        discovery.start()
        listeners.single().onServiceFound(service("host"))
        drain()
        resolutions.single().second.onServiceResolved(service("host", 0))
        drain()
        assertTrue(discovery.results.isEmpty())
    }

    private fun resolve(index: Int) {
        val (info, listener) = resolutions[index]
        listener.onServiceResolved(service(info.serviceName))
        drain()
    }

    private fun drain() = shadowOf(Looper.getMainLooper()).idle()

    private fun service(name: String, port: Int = 9876) = NsdServiceInfo().apply {
        serviceName = name
        serviceType = "_stagewand._tcp."
        host = InetAddress.getByAddress(byteArrayOf(192.toByte(), 168.toByte(), 1, 2))
        this.port = port
    }
}
