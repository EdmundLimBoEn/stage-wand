package systems.edmundlim.stagewand.protocol

import kotlin.math.max
import kotlin.math.min

class CommandQueue {
    private val pending = ArrayDeque<Command>()
    val isEmpty: Boolean get() = pending.isEmpty()
    val count: Int get() = pending.size

    fun append(command: Command) {
        val last = pending.lastOrNull()
        if (command is Command.Move && last is Command.Move) {
            val x = last.dx + command.dx
            val y = last.dy + command.dy
            if (x.isFinite() && y.isFinite()) {
                pending.removeLast()
                pending.addLast(Command.Move(x, y))
                return
            }
        } else if (command is Command.Scroll && last is Command.Scroll) {
            val x = last.dx + command.dx
            val y = last.dy + command.dy
            if (x.isFinite() && y.isFinite()) {
                pending.removeLast()
                pending.addLast(Command.Scroll(x, y))
                return
            }
        }
        pending.addLast(command)
    }

    fun popFirst(): Command? {
        if (pending.isEmpty()) return null
        val command = pending.removeFirst()
        if (command is Command.Move) {
            val dx = min(400.0, max(-400.0, command.dx))
            val dy = min(400.0, max(-400.0, command.dy))
            if (command.dx != dx || command.dy != dy) {
                pending.addFirst(Command.Move(command.dx - dx, command.dy - dy))
            }
            return Command.Move(dx, dy)
        }
        return command
    }

    fun removeAll() {
        pending.clear()
    }
}
