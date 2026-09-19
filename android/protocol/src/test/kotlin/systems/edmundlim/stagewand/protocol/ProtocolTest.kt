package systems.edmundlim.stagewand.protocol

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue
import kotlin.test.fail

class ProtocolTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun fixturesRoundTrip() {
        val text = javaClass.classLoader!!.getResource("protocol-fixtures.json")!!.readText()
        val root = json.parseToJsonElement(text).jsonObject
        for (frame in root["commands"]!!.jsonArray) {
            val encoded = frame.toString()
            val command = Command.decode(encoded) ?: fail("decode $encoded")
            val again = Command.decode(command.encode()) ?: fail("redecode")
            assertEquals(command, again)
        }
        for (frame in root["replies"]!!.jsonArray) {
            val encoded = frame.toString()
            val reply = Reply.decode(encoded) ?: fail("decode $encoded")
            assertEquals(reply, Reply.decode(reply.encode()))
        }
        for (frame in root["invalidCommands"]!!.jsonArray) {
            val encoded = frame.jsonPrimitive.content
            assertNull(Command.decode(encoded), encoded)
        }
    }
}

class SquareUnlockTest {
    private fun trace(duration: Double = 2.0): SquareUnlock {
        val recognizer = SquareUnlock()
        for (step in 0..40) {
            val p = step / 10.0
            val point = when {
                p <= 1 -> p to 0.0
                p <= 2 -> 1.0 to (p - 1)
                p <= 3 -> (3 - p) to 1.0
                else -> 0.0 to (4 - p)
            }
            recognizer.add(point.first, point.second, duration * p / 4)
        }
        return recognizer
    }

    @Test
    fun recognizerCases() {
        assertTrue(trace().finish(0.0, 0.0, 2.0), "Valid square rejected")
        assertFalse(trace(0.2).finish(0.0, 0.0, 0.2), "Fast gesture accepted")
        val tap = SquareUnlock()
        tap.add(0.0, 0.0, 0.0)
        assertFalse(tap.finish(0.0, 0.0, 2.0), "Tap accepted")
        val partial = SquareUnlock()
        for (i in 0..10) partial.add(i / 10.0, 0.0, i / 10.0)
        assertFalse(partial.finish(1.0, 0.0, 2.0), "Partial accepted")
        val diagonal = SquareUnlock()
        diagonal.add(0.0, 0.0, 0.0)
        diagonal.add(0.5, 0.5, 1.0)
        assertFalse(diagonal.finish(0.0, 0.0, 2.0), "Diagonal accepted")
        val wrongStart = SquareUnlock()
        wrongStart.add(1.0, 0.0, 0.0)
        assertTrue(wrongStart.failed, "Wrong start accepted")
        diagonal.reset()
        assertTrue(!diagonal.failed && diagonal.progress == 0.0, "Reset retained trace")
        assertFalse(diagonal.finish(0.0, 0.0, 3.0), "Reset accepted stale gesture")
        val scribble = SquareUnlock()
        scribble.add(0.0, 0.0, 0.0)
        for (i in 1..20) scribble.add(if (i % 2 == 0) 0.0 else 0.1, 0.0, i / 10.0)
        assertTrue(scribble.failed, "Scribble accepted")
    }
}

class CommandQueueTest {
    @Test
    fun coalescesAndSplits() {
        val queue = CommandQueue()
        repeat(1000) { queue.append(Command.Move(1.0, -2.0)) }
        assertEquals(1, queue.count)
        queue.append(Command.Key(KeyName.Right))
        queue.append(Command.Move(10.0, 20.0))
        queue.append(Command.Click(Button.Left))
        queue.append(Command.Scroll(1.0, 3.0))
        queue.append(Command.Scroll(2.0, 4.0))
        var x = 0.0
        var y = 0.0
        while (true) {
            val command = queue.popFirst() ?: fail("empty")
            if (command is Command.Key) break
            val move = command as? Command.Move ?: fail("Reordered barrier")
            assertTrue(kotlin.math.abs(move.dx) <= 400 && kotlin.math.abs(move.dy) <= 400)
            x += move.dx
            y += move.dy
        }
        assertEquals(1000.0, x)
        assertEquals(-2000.0, y)
        val afterKey = queue.popFirst() as Command.Move
        assertEquals(10.0, afterKey.dx)
        assertEquals(20.0, afterKey.dy)
        assertTrue(queue.popFirst() is Command.Click)
        val scroll = queue.popFirst() as Command.Scroll
        assertEquals(3.0, scroll.dx)
        assertEquals(7.0, scroll.dy)
        assertTrue(queue.isEmpty)
        queue.append(Command.Move(1.0, 1.0))
        queue.removeAll()
        assertNull(queue.popFirst())
    }
}

class ConnectionURLTest {
    @Test
    fun acceptedForms() {
        val cases = listOf(
            "mac.local" to "ws://mac.local:8787/",
            "192.168.1.2:9000" to "ws://192.168.1.2:9000/",
            "http://mac.local" to "ws://mac.local:8787/",
            "ws://mac.local:8787/hello?q=1" to "ws://mac.local:8787/hello?q=1",
            "wss://tunnel.example/secret%2Fpath?a=1&b=%26" to "wss://tunnel.example:443/secret%2Fpath?a=1&b=%26",
            "https://tunnel.example/secret?token=abc" to "wss://tunnel.example:443/secret?token=abc",
            "https://tunnel.example:8443/" to "wss://tunnel.example:8443/",
            " [::1]:8787 " to "ws://[::1]:8787/"
        )
        for ((input, expected) in cases) {
            assertEquals(expected, ConnectionURL.parse(input), input)
        }
    }

    @Test
    fun rejectedForms() {
        val invalid = listOf(
            "", "ws://", "ftp://example.com", "https://u:p@example.com",
            "wss://example.com/#fragment", "wss://example.com:0", "wss://example.com:65536",
            "wss://example.com:abc", "wss://example.com:", "wss://bad host/"
        )
        for (input in invalid) {
            assertNull(ConnectionURL.parse(input), input)
        }
    }

    @Test
    fun deepLink() {
        val encoded = java.net.URLEncoder.encode("wss://tunnel.example/secret?token=a&b=c", Charsets.UTF_8)
        val url = "stagewand://connect?url=$encoded"
        assertEquals(
            "wss://tunnel.example:443/secret?token=a&b=c",
            ConnectionURL.pairingDestination(url)
        )
        val invalid = listOf(
            "stagewand://other?url=wss://example.com",
            "stagewand://connect?url=wss://a&url=wss://b",
            "stagewand://connect?code=123456",
            "stagewand://connect?url=ftp://example.com"
        )
        for (input in invalid) {
            assertNull(ConnectionURL.pairingDestination(input), input)
        }
    }
}
