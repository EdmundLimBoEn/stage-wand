package systems.edmundlim.stagewand.protocol

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class CommandWriterTest {
    @Test fun movementCoalescesWhileWriteIsInFlightAndPreservesClickOrder() {
        val sent = mutableListOf<Command>()
        val writer = CommandWriter { sent.add(it); true }
        writer.enqueue(Command.Auth("0000"))
        repeat(1000) { writer.enqueue(Command.Move(1.0, -1.0)) }
        writer.enqueue(Command.Click(Button.Left))
        assertEquals(1, sent.size)
        assertEquals(3, writer.count)
        while (writer.count > 0) writer.completed()
        assertEquals(listOf(Command.Auth("0000"), Command.Move(400.0, -400.0), Command.Move(400.0, -400.0), Command.Move(200.0, -200.0), Command.Click(Button.Left)), sent)
    }

    @Test fun busyWriteRetriesWithoutLosingDistance() {
        val sent = mutableListOf<Command>()
        var available = false
        val writer = CommandWriter { if (available) { sent.add(it); true } else false }
        writer.enqueue(Command.Move(2.0, 3.0))
        repeat(1000) { writer.enqueue(Command.Move(1.0, 0.0)) }
        assertTrue(writer.needsRetry)
        assertEquals(2, writer.count)
        available = true
        writer.flush()
        while (writer.count > 0) writer.completed()
        assertEquals(1002.0, sent.filterIsInstance<Command.Move>().sumOf { it.dx })
        assertEquals(3.0, sent.filterIsInstance<Command.Move>().sumOf { it.dy })
    }

    @Test fun clearDiscardsStaleCommandsAndOverflowIsBounded() {
        val sent = mutableListOf<Command>()
        val writer = CommandWriter { sent.add(it); true }
        repeat(64) { assertTrue(writer.enqueue(Command.Key(KeyName.Right))) }
        assertFalse(writer.enqueue(Command.Key(KeyName.Right)))
        assertEquals(0, writer.count)
        writer.enqueue(Command.Auth("1234"))
        writer.clear()
        writer.completed()
        assertEquals(listOf(Command.Key(KeyName.Right), Command.Auth("1234")), sent)
    }
}
