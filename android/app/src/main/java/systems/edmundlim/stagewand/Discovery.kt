package systems.edmundlim.stagewand

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper

data class HostService(val name: String, val host: String, val port: Int)

class Discovery(context: Context) {
    var onResults: (() -> Unit)? = null
    var onDenied: (() -> Unit)? = null
    var results: List<HostService> = emptyList()
        private set

    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager
    private val main = Handler(Looper.getMainLooper())
    private var started = false
    private val found = LinkedHashMap<String, NsdServiceInfo>()

    private val listener = object : NsdManager.DiscoveryListener {
        override fun onStartDiscoveryFailed(serviceType: String?, errorCode: Int) {
            main.post { onDenied?.invoke() }
        }
        override fun onStopDiscoveryFailed(serviceType: String?, errorCode: Int) {}
        override fun onDiscoveryStarted(serviceType: String?) {}
        override fun onDiscoveryStopped(serviceType: String?) {}
        override fun onServiceFound(serviceInfo: NsdServiceInfo) {
            nsd.resolveService(serviceInfo, object : NsdManager.ResolveListener {
                override fun onResolveFailed(serviceInfo: NsdServiceInfo?, errorCode: Int) {}
                override fun onServiceResolved(resolved: NsdServiceInfo) {
                    main.post {
                        found[resolved.serviceName] = resolved
                        publish()
                    }
                }
            })
        }
        override fun onServiceLost(serviceInfo: NsdServiceInfo) {
            main.post {
                found.remove(serviceInfo.serviceName)
                publish()
            }
        }
    }

    fun start() {
        if (started) return
        started = true
        try {
            nsd.discoverServices("_stagewand._tcp.", NsdManager.PROTOCOL_DNS_SD, listener)
        } catch (_: RuntimeException) {
            started = false
            onDenied?.invoke()
        }
    }

    fun stop() {
        if (!started) return
        started = false
        try {
            nsd.stopServiceDiscovery(listener)
        } catch (_: RuntimeException) {
        }
        found.clear()
        results = emptyList()
    }

    private fun publish() {
        results = found.values.mapNotNull { info ->
            val host = info.host?.hostAddress ?: return@mapNotNull null
            HostService(info.serviceName ?: host, host, info.port)
        }.sortedBy { it.name }
        onResults?.invoke()
    }
}
