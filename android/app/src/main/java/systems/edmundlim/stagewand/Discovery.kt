package systems.edmundlim.stagewand

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Handler
import android.os.Looper
import java.util.ArrayDeque

data class HostService(val name: String, val host: String, val port: Int)

class Discovery internal constructor(
    private val nsd: NsdManager,
    private val main: Handler,
) {
    constructor(context: Context) : this(
        context.applicationContext.getSystemService(Context.NSD_SERVICE) as NsdManager,
        Handler(Looper.getMainLooper()),
    )

    var onResults: (() -> Unit)? = null
    var onDenied: (() -> Unit)? = null
    var results: List<HostService> = emptyList()
        private set

    private class Resolution(val info: NsdServiceInfo, val generation: Long) {
        var attempts = 0
    }

    private var generation = 0L
    private var listener: NsdManager.DiscoveryListener? = null
    private val present = HashMap<String, Resolution>()
    private val found = LinkedHashMap<String, HostService>()
    private val pending = ArrayDeque<Resolution>()
    private var resolving: Resolution? = null

    fun start() = onMain {
        if (listener != null) return@onMain
        val session = ++generation
        val nextListener = object : NsdManager.DiscoveryListener {
            private var stopRetries = 0
            private var stopped = false

            override fun onStartDiscoveryFailed(serviceType: String?, errorCode: Int) {
                main.post {
                    if (!isCurrent(session)) return@post
                    listener = null
                    clearResults()
                    onDenied?.invoke()
                }
            }

            override fun onStopDiscoveryFailed(serviceType: String?, errorCode: Int) {
                main.post {
                    if (stopped || stopRetries >= 2) return@post
                    stopRetries++
                    main.postDelayed({
                        if (!stopped) stopDiscovery(this)
                    }, 250L * stopRetries)
                }
            }
            override fun onDiscoveryStarted(serviceType: String?) {}
            override fun onDiscoveryStopped(serviceType: String?) {
                main.post {
                    stopped = true
                    if (!isCurrent(session)) return@post
                    listener = null
                    clearResults()
                }
            }

            override fun onServiceFound(serviceInfo: NsdServiceInfo) {
                main.post {
                    if (!isCurrent(session) || serviceInfo.serviceName.isNullOrBlank()) return@post
                    val request = Resolution(serviceInfo, session)
                    present[serviceInfo.serviceName] = request
                    pending.removeAll { it.info.serviceName == serviceInfo.serviceName }
                    pending.addLast(request)
                    resolveNext()
                }
            }

            override fun onServiceLost(serviceInfo: NsdServiceInfo) {
                main.post {
                    if (!isCurrent(session)) return@post
                    present.remove(serviceInfo.serviceName)
                    pending.removeAll { it.info.serviceName == serviceInfo.serviceName }
                    if (found.remove(serviceInfo.serviceName) != null) publish()
                }
            }
        }
        listener = nextListener
        try {
            nsd.discoverServices("_stagewand._tcp.", NsdManager.PROTOCOL_DNS_SD, nextListener)
        } catch (_: RuntimeException) {
            listener = null
            clearResults()
            onDenied?.invoke()
        }
    }

    fun stop() = onMain {
        val previous = listener
        listener = null
        clearResults()
        if (previous != null) stopDiscovery(previous)
    }

    private fun stopDiscovery(previous: NsdManager.DiscoveryListener) {
        try {
            nsd.stopServiceDiscovery(previous)
        } catch (_: RuntimeException) {
        }
    }

    private fun resolveNext() {
        // Legacy Android permits only one resolution at a time, even across discovery restarts.
        if (resolving != null) return
        while (pending.isNotEmpty()) {
            val request = pending.removeFirst()
            if (!isPresent(request)) continue
            resolving = request
            request.attempts++
            try {
                nsd.resolveService(request.info, object : NsdManager.ResolveListener {
                    override fun onResolveFailed(serviceInfo: NsdServiceInfo?, errorCode: Int) {
                        main.post { complete(request, null, retry = true) }
                    }

                    override fun onServiceResolved(resolved: NsdServiceInfo) {
                        main.post { complete(request, resolved, retry = false) }
                    }
                })
            } catch (_: RuntimeException) {
                complete(request, null, retry = false)
            }
            return
        }
    }

    private fun complete(request: Resolution, resolved: NsdServiceInfo?, retry: Boolean) {
        if (resolving !== request) return
        resolving = null
        if (isPresent(request)) {
            val address = resolved?.host?.hostAddress
            if (resolved != null && !address.isNullOrBlank() && resolved.port in 1..65535) {
                found[request.info.serviceName] = HostService(
                    resolved.serviceName?.takeIf { it.isNotBlank() } ?: request.info.serviceName,
                    address,
                    resolved.port,
                )
                publish()
            } else if (retry && request.attempts < 3) {
                main.postDelayed({
                    if (isPresent(request)) {
                        pending.addLast(request)
                        resolveNext()
                    }
                }, 250L * request.attempts)
            }
        }
        resolveNext()
    }

    private fun isCurrent(session: Long) = listener != null && generation == session

    private fun isPresent(request: Resolution) =
        isCurrent(request.generation) && present[request.info.serviceName] === request

    private fun clearResults() {
        present.clear()
        pending.clear()
        found.clear()
        // An in-flight legacy resolution cannot be cancelled on Android 8–13. Its callback
        // releases the slot, but isPresent prevents it from reviving a stopped/lost service.
        if (results.isNotEmpty()) publish()
    }

    private fun publish() {
        results = found.values.sortedBy { it.name }
        onResults?.invoke()
    }

    private inline fun onMain(crossinline action: () -> Unit) {
        if (Looper.myLooper() == main.looper) action() else main.post { action() }
    }
}
