package systems.edmundlim.stagewand

import android.os.Handler
import android.os.Looper
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.CommandQueue
import systems.edmundlim.stagewand.protocol.ConnectionURL
import systems.edmundlim.stagewand.protocol.Reply
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class Link(
    private val discovery: Discovery
) {
    enum class State {
        Searching, Connecting, EnterCode, Authed, Disconnected, LocalNetworkDenied
    }

    var state: State = State.Searching
        private set
    var hostName: String? = null
        private set
    var route: String = "Nearby"
        private set
    var onChange: (() -> Unit)? = null

    private val main = Handler(Looper.getMainLooper())
    private val client = OkHttpClient.Builder()
        .pingInterval(0, TimeUnit.SECONDS)
        .retryOnConnectionFailure(false)
        .build()
    private var socket: WebSocket? = null
    private var settings = Settings()
    private val buffered = ArrayDeque<Command>()
    private val outgoing = CommandQueue()
    private val generation = AtomicInteger(0)
    private var sending = false

    init {
        discovery.onResults = {
            if (state == State.Searching || state == State.Disconnected) connect()
        }
        discovery.onDenied = {
            if (settings.manualHost.isEmpty() && state != State.Authed) {
                resetConnection()
                state = State.LocalNetworkDenied
                notifyChanged()
            }
        }
    }

    fun update(settings: Settings) {
        val reconnect = settings.pairingCode != this.settings.pairingCode ||
            settings.manualHost.trim() != this.settings.manualHost.trim()
        this.settings = settings
        if (!reconnect) return
        resetConnection()
        state = if (settings.pairingCode.isEmpty()) State.EnterCode else State.Searching
        notifyChanged()
        if (settings.pairingCode.isNotEmpty()) connect()
    }

    fun start() {
        if (settings.pairingCode.isEmpty()) {
            state = State.EnterCode
            notifyChanged()
            return
        }
        connect()
    }

    fun send(command: Command) {
        when (command) {
            is Command.Auth -> return
            is Command.Move -> if (state == State.Authed && (command.dx != 0.0 || command.dy != 0.0) && command.dx.isFinite() && command.dy.isFinite()) enqueue(command)
            is Command.Scroll -> if (state == State.Authed && command.dx.isFinite() && command.dy.isFinite()) enqueue(command)
            is Command.Key, is Command.Click, is Command.Chord -> {
                if (state == State.Authed) enqueue(command)
                else {
                    if (buffered.size == 8) buffered.removeFirst()
                    buffered.addLast(command)
                }
            }
        }
    }

    private fun connect() {
        if (socket != null) return
        if (settings.pairingCode.isEmpty()) {
            state = State.EnterCode
            notifyChanged()
            return
        }
        val manual = settings.manualHost.trim()
        if (manual.isNotEmpty()) {
            val url = ConnectionURL.parse(manual)
            if (url == null) {
                hostName = null
                disconnected()
                return
            }
            hostName = hostOf(url)
            route = if (url.startsWith("wss://")) "Secure link" else "Direct"
            open(url)
            return
        }
        discovery.start()
        val result = discovery.results.firstOrNull()
        if (result == null) {
            state = State.Searching
            notifyChanged()
            return
        }
        hostName = result.name
        route = "Nearby"
        open("ws://${formatHost(result.host)}:${result.port}/")
    }

    private fun open(url: String) {
        val id = generation.incrementAndGet()
        state = State.Connecting
        notifyChanged()
        val request = Request.Builder().url(url).build()
        socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                main.post {
                    if (generation.get() != id) return@post
                    enqueue(Command.Auth(settings.pairingCode))
                }
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                main.post {
                    if (generation.get() != id) return@post
                    when (val reply = Reply.decode(text)) {
                        is Reply.Status -> {
                            state = State.Authed
                            val pending = buffered.toList()
                            buffered.clear()
                            pending.forEach(::enqueue)
                            notifyChanged()
                        }
                        is Reply.Bye -> {
                            resetConnection()
                            state = if (reply.reason == "badauth") State.EnterCode else State.Disconnected
                            notifyChanged()
                            if (state == State.Disconnected) scheduleReconnect()
                        }
                        null -> {}
                    }
                }
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                main.post { if (generation.get() == id) disconnected() }
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                main.post { if (generation.get() == id) disconnected() }
            }
        })
        main.postDelayed({
            if (generation.get() == id && state != State.Authed) disconnected()
        }, 10_000)
    }

    private fun enqueue(command: Command) {
        outgoing.append(command)
        flush()
    }

    private fun flush() {
        val webSocket = socket ?: return
        if (sending) return
        sending = true
        while (!outgoing.isEmpty) {
            val command = outgoing.popFirst() ?: break
            if (!webSocket.send(command.encode())) {
                sending = false
                disconnected()
                return
            }
        }
        sending = false
    }

    private fun disconnected() {
        resetConnection()
        state = State.Disconnected
        notifyChanged()
        scheduleReconnect()
    }

    private fun scheduleReconnect() {
        val id = generation.get()
        main.postDelayed({
            if (generation.get() == id) connect()
        }, 1000)
    }

    private fun resetConnection() {
        generation.incrementAndGet()
        socket?.cancel()
        socket = null
        sending = false
        outgoing.removeAll()
    }

    private fun notifyChanged() {
        onChange?.invoke()
    }

    private fun hostOf(url: String): String? = try {
        java.net.URI(url).host
    } catch (_: Exception) {
        null
    }

    private fun formatHost(host: String): String =
        if (":" in host && !host.startsWith("[")) "[$host]" else host
}
