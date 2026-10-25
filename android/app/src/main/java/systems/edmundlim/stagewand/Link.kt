package systems.edmundlim.stagewand

import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.CommandQueue
import systems.edmundlim.stagewand.protocol.ConnectionURL
import systems.edmundlim.stagewand.protocol.Reply
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.nio.charset.StandardCharsets
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger

class Link(
    private val discovery: Discovery,
    private val bluetooth: BluetoothClient,
    private val client: OkHttpClient = OkHttpClient.Builder()
        .pingInterval(15, TimeUnit.SECONDS)
        .retryOnConnectionFailure(false)
        .build()
) {
    enum class State {
        Searching, Connecting, EnterCode, Authed, Disconnected, LocalNetworkDenied, BluetoothDenied
    }

    var state: State = State.Searching
        private set
    var hostName: String? = null
        private set
    var route: String = "Nearby"
        private set
    var failureMessage: String? = null
        private set
    var onChange: (() -> Unit)? = null

    private val main = Handler(Looper.getMainLooper())
    private var socket: WebSocket? = null
    private var closed = false
    private var started = false
    private var reconnectSuppressed = false
    private var reconnectAttempt = 0
    private var live = false
    private var settings = Settings()
    private val outgoing = CommandQueue()
    private val generation = AtomicInteger(0)
    private var sending = false
    private var backlogSince: Long? = null
    private val backlogCheck = object : Runnable {
        override fun run() {
            val ws = socket ?: return
            if (!checkBacklog(ws)) return
            if (backlogSince != null) main.postDelayed(this, 25)
        }
    }

    private companion object {
        const val MAX_BACKLOG_BYTES = 4 * 1024L
        const val MAX_BACKLOG_AGE_MS = 250L
    }

    init {
        discovery.onResults = {
            if (settings.transport != "bluetooth" && settings.manualHost.isBlank() &&
                (state == State.Searching || state == State.Disconnected)) connect()
        }
        discovery.onDenied = {
            if (settings.manualHost.isBlank() && settings.transport != "bluetooth" && state != State.Authed) {
                resetConnection()
                state = State.LocalNetworkDenied
                notifyChanged()
            }
        }
    }

    fun update(settings: Settings) {
        if (closed) return
        val reconnect = settings.pairingCode != this.settings.pairingCode ||
            settings.manualHost.trim() != this.settings.manualHost.trim() ||
            settings.transport != this.settings.transport
        this.settings = settings
        if (!reconnect) return
        reconnectSuppressed = false
        resetConnection()
        reconnectAttempt = 0
        failureMessage = null
        state = if (!hasPairingCode()) State.EnterCode else State.Searching
        notifyChanged()
        if (started && hasPairingCode()) connect()
    }

    fun markBluetoothDenied() {
        started = true
        resetConnection()
        failureMessage = BluetoothClient.Failure.PermissionDenied.message
        state = State.BluetoothDenied
        notifyChanged()
    }

    fun start() {
        if (closed) return
        reconnectSuppressed = false
        started = true
        if (!hasPairingCode()) {
            failureMessage = null
            state = State.EnterCode
            notifyChanged()
            return
        }
        connect()
    }

    /** Returns true when the command was queued for the computer. */
    fun send(command: Command): Boolean {
        if (closed) return false
        val accepted = when (command) {
            is Command.Auth -> false
            is Command.Move -> state == State.Authed && (command.dx != 0.0 || command.dy != 0.0) && command.dx.isFinite() && command.dy.isFinite()
            is Command.Scroll -> state == State.Authed && command.dx.isFinite() && command.dy.isFinite()
            is Command.Key, is Command.Click, is Command.Chord -> state == State.Authed
        }
        if (accepted) enqueue(command)
        return accepted
    }

    private fun connect() {
        if (closed || !started || live || reconnectSuppressed) return
        if (!hasPairingCode()) {
            failureMessage = null
            state = State.EnterCode
            notifyChanged()
            return
        }
        val manual = settings.manualHost.trim()
        if (manual.isNotEmpty()) {
            discovery.stop()
            val url = ConnectionURL.parse(manual)
            if (url == null) {
                hostName = null
                resetConnection()
                state = State.Disconnected
                failureMessage = "Enter a valid host address in Settings."
                notifyChanged()
                return
            }
            hostName = hostOf(url)
            route = if (url.startsWith("wss://")) "Secure link" else "Direct"
            open(url)
            return
        }
        if (settings.transport == "bluetooth") {
            discovery.stop()
            openBluetooth()
            return
        }
        state = State.Searching
        discovery.start()
        if (state == State.LocalNetworkDenied) return
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

    private fun openBluetooth() {
        live = true
        route = "Bluetooth"
        hostName = null
        val id = generation.incrementAndGet()
        bluetooth.onName = onName@{
            if (generation.get() != id) return@onName
            hostName = it
            notifyChanged()
        }
        state = State.Connecting
        failureMessage = null
        notifyChanged()
        bluetooth.connect(
            onReady = {
                main.post {
                    if (generation.get() != id) return@post
                    enqueue(Command.Auth(settings.pairingCode))
                }
            },
            onData = { bytes ->
                main.post {
                    if (generation.get() != id) return@post
                    handleReply(decodeBluetoothReply(bytes))
                }
            },
            onFail = { failure ->
                if (generation.get() == id) {
                    resetConnection()
                    state = if (failure == BluetoothClient.Failure.PermissionDenied) State.BluetoothDenied else State.Disconnected
                    failureMessage = failure.message
                    notifyChanged()
                    if (failure.retryable) scheduleReconnect()
                }
            }
        )
        main.postDelayed({
            if (generation.get() == id && state != State.Authed) disconnected()
        }, 20_000)
    }

    private fun open(url: String) {
        live = true
        val id = generation.incrementAndGet()
        state = State.Connecting
        failureMessage = null
        notifyChanged()
        val request = try {
            Request.Builder().url(url).build()
        } catch (_: IllegalArgumentException) {
            resetConnection()
            state = State.Disconnected
            failureMessage = "This host address is not supported. Enter an IPv4 address or hostname in Settings."
            notifyChanged()
            return
        }
        socket = client.newWebSocket(request, object : WebSocketListener() {
            // Reset removes queued callbacks, so each connection needs its own counter.
            private val pendingMessages = AtomicInteger(0)

            override fun onOpen(webSocket: WebSocket, response: Response) {
                main.post {
                    if (generation.get() != id) return@post
                    enqueue(Command.Auth(settings.pairingCode))
                }
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                if (generation.get() != id) return
                if (text.length > 16 * 1024 || text.toByteArray(StandardCharsets.UTF_8).size > 16 * 1024) {
                    webSocket.close(1009, "Reply exceeds 16 KiB")
                    return
                }
                if (pendingMessages.incrementAndGet() > 64) {
                    pendingMessages.decrementAndGet()
                    return
                }
                if (!main.post {
                    try {
                        if (generation.get() == id) handleReply(Reply.decode(text))
                    } finally {
                        pendingMessages.decrementAndGet()
                    }
                }) pendingMessages.decrementAndGet()
            }
            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(code, reason)
                main.post { if (generation.get() == id) disconnected() }
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

    private fun handleReply(reply: Reply?) {
        when (reply) {
            is Reply.Status -> {
                if (state != State.Connecting) return
                state = State.Authed
                failureMessage = null
                reconnectAttempt = 0
                notifyChanged()
            }
            is Reply.Bye -> {
                resetConnection()
                reconnectSuppressed = reply.reason == "displaced"
                state = if (reply.reason == "badauth" || reply.reason == "kicked") State.EnterCode else State.Disconnected
                failureMessage = if (reply.reason == "badauth") "Pairing code rejected. Enter the current code shown on the host. After repeated attempts, wait 30 seconds or choose Disconnect & new code on the host." else null
                notifyChanged()
                if (state == State.Disconnected) scheduleReconnect()
            }
            null -> {}
        }
    }

    private fun enqueue(command: Command) {
        if (!outgoing.append(command)) {
            slowConnection()
            return
        }
        flush()
    }

    private fun flush() {
        if (sending) return
        sending = true
        while (!outgoing.isEmpty) {
            val command = outgoing.popFirst() ?: break
            val id = generation.get()
            val ws = socket
            val sent = if (ws != null) {
                val payload = command.encode()
                if (!checkBacklog(ws, payload.toByteArray(StandardCharsets.UTF_8).size)) return
                if (backlogSince == null) {
                    backlogSince = SystemClock.elapsedRealtime()
                    main.removeCallbacks(backlogCheck)
                    main.postDelayed(backlogCheck, 25)
                }
                ws.send(payload)
            } else {
                bluetooth.send(command)
            }
            if (!sent) {
                sending = false
                if (generation.get() == id) disconnected()
                return
            }
        }
        sending = false
    }

    // queueSize includes OkHttp's in-flight write. Only an observed empty queue
    // resets the deadline; partial progress must not extend stale controls' lifetime.
    private fun checkBacklog(ws: WebSocket, nextBytes: Int = 0): Boolean {
        val queued = ws.queueSize()
        if (queued == 0L) backlogSince = null
        val since = backlogSince
        if (queued + nextBytes > MAX_BACKLOG_BYTES ||
            (since != null && SystemClock.elapsedRealtime() - since >= MAX_BACKLOG_AGE_MS)) {
            slowConnection()
            return false
        }
        return true
    }

    private fun slowConnection() {
        resetConnection()
        state = State.Disconnected
        failureMessage = "Connection is too slow to keep up. Reconnecting…"
        notifyChanged()
        scheduleReconnect()
    }

    private fun disconnected() {
        resetConnection()
        state = State.Disconnected
        failureMessage = "Disconnected. Reconnecting…"
        notifyChanged()
        scheduleReconnect()
    }

    private fun scheduleReconnect() {
        if (closed || reconnectSuppressed) return
        val id = generation.get()
        val delay = (1_000L shl reconnectAttempt.coerceAtMost(5)).coerceAtMost(30_000L)
        reconnectAttempt = (reconnectAttempt + 1).coerceAtMost(5)
        main.postDelayed({
            if (generation.get() == id) connect()
        }, delay)
    }

    fun close() {
        if (closed) return
        closed = true
        onChange = null
        resetConnection()
        main.removeCallbacksAndMessages(null)
        discovery.onResults = null
        discovery.onDenied = null
        discovery.stop()
        bluetooth.onName = null
        client.dispatcher.executorService.shutdown()
        client.connectionPool.evictAll()
    }

    private fun resetConnection() {
        generation.incrementAndGet()
        main.removeCallbacksAndMessages(null)
        live = false
        socket?.cancel()
        socket = null
        bluetooth.cancel()
        sending = false
        outgoing.removeAll()
        backlogSince = null
    }

    private fun decodeBluetoothReply(bytes: ByteArray): Reply? = try {
        val text = StandardCharsets.UTF_8.newDecoder()
            .onMalformedInput(CodingErrorAction.REPORT)
            .onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(bytes))
        Reply.decode(text.toString())
    } catch (_: CharacterCodingException) {
        null
    }

    private fun hasPairingCode(): Boolean = settings.pairingCode.length == 4 &&
        settings.pairingCode.all { it in '0'..'9' }

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
