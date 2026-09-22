package systems.edmundlim.stagewand.protocol

import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.max
import kotlin.math.min

class CommandQueue {
    private val pending = ArrayDeque<Command>()
    val isEmpty: Boolean get() = pending.isEmpty()
    val count: Int get() = pending.size

    fun append(command: Command): Boolean {
        if (writeCount(command) == null) {
            removeAll()
            return false
        }
        val last = pending.lastOrNull()
        if (command is Command.Move && last is Command.Move) {
            val x = last.dx + command.dx
            val y = last.dy + command.dy
            if (x.isFinite() && y.isFinite()) {
                pending.removeLast()
                pending.addLast(Command.Move(x, y))
                return checkCapacity()
            }
        } else if (command is Command.Scroll && last is Command.Scroll) {
            val x = last.dx + command.dx
            val y = last.dy + command.dy
            if (x.isFinite() && y.isFinite()) {
                pending.removeLast()
                pending.addLast(Command.Scroll(x, y))
                return checkCapacity()
            }
        }
        pending.addLast(command)
        return checkCapacity()
    }

    private fun checkCapacity(): Boolean {
        var writes = 0
        for (command in pending) {
            val count = writeCount(command)
            if (count == null || writes + count > 64) {
                removeAll()
                return false
            }
            writes += count
        }
        return true
    }

    private fun writeCount(command: Command): Int? {
        val delta = when (command) {
            is Command.Move -> command.dx to command.dy
            is Command.Scroll -> command.dx to command.dy
            else -> return 1
        }
        val (x, y) = delta
        if (!x.isFinite() || !y.isFinite() || abs(x) > 25_600 || abs(y) > 25_600) return null
        return max(1, ceil(max(abs(x), abs(y)) / 400).toInt())
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
        if (command is Command.Scroll) {
            val dx = min(400.0, max(-400.0, command.dx))
            val dy = min(400.0, max(-400.0, command.dy))
            if (command.dx != dx || command.dy != dy) {
                pending.addFirst(Command.Scroll(command.dx - dx, command.dy - dy))
            }
            return Command.Scroll(dx, dy)
        }
        return command
    }

    fun removeAll() {
        pending.clear()
    }
}
