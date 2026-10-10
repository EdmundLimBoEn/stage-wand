package systems.edmundlim.stagewand.protocol

class CommandWriter(private val write: (Command) -> Boolean) {
    private val queue = CommandQueue()
    private var pending: Command? = null
    private var inFlight = false
    val count: Int get() = queue.count + (if (pending != null) 1 else 0) + (if (inFlight) 1 else 0)
    val needsRetry: Boolean get() = pending != null && !inFlight

    fun enqueue(command: Command): Boolean {
        if (!queue.append(command) || count > 64) {
            clear()
            return false
        }
        flush()
        return true
    }

    fun flush() {
        if (inFlight) return
        val command = pending ?: queue.popFirst() ?: return
        pending = command
        if (write(command)) {
            pending = null
            inFlight = true
        }
    }

    fun completed() {
        inFlight = false
        flush()
    }

    fun clear() {
        queue.removeAll()
        pending = null
        inFlight = false
    }
}
