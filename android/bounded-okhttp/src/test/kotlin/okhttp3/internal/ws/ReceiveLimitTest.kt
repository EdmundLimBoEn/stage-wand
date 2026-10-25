package okhttp3.internal.ws

import java.net.ProtocolException
import java.util.Random
import java.util.zip.Deflater
import okhttp3.Request
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okhttp3.internal.concurrent.TaskRunner
import okio.Buffer
import okio.ByteString
import org.junit.Assert.*
import org.junit.Test

class ReceiveLimitTest {
    private val limit = 16 * 1024

    private class Messages : WebSocketReader.FrameCallback {
        val sizes = mutableListOf<Int>()
        var pings = 0
        override fun onReadMessage(text: String) { sizes += text.toByteArray().size }
        override fun onReadMessage(bytes: ByteString) { sizes += bytes.size }
        override fun onReadPing(payload: ByteString) { pings++ }
        override fun onReadPong(payload: ByteString) {}
        override fun onReadClose(code: Int, reason: String) {}
    }

    private fun Buffer.header(opcode: Int, length: Long, final: Boolean = true, compressed: Boolean = false): Buffer {
        writeByte(opcode or (if (final) 0x80 else 0) or (if (compressed) 0x40 else 0))
        when {
            length < 126 -> writeByte(length.toInt())
            length <= 65535 -> { writeByte(126); writeShort(length.toInt()) }
            else -> { writeByte(127); writeLong(length) }
        }
        return this
    }

    private fun reader(wire: Buffer, messages: Messages) =
        WebSocketReader(true, wire, messages, true, false)

    private fun compressed(deflater: Deflater, count: Int): ByteArray {
        deflater.setInput(ByteArray(count) { 65 })
        val bytes = ByteArray(count + 128)
        val size = deflater.deflate(bytes, 0, bytes.size, Deflater.SYNC_FLUSH)
        assertTrue(deflater.needsInput())
        // RFC 7692 omits the trailing 00 00 ff ff.
        return bytes.copyOf(size - 4)
    }

    @Test fun oversizedHeadersFailBeforeAnyPayloadReadForBothOpcodes() {
        for (opcode in listOf(1, 2)) for (size in listOf(limit + 1L, Long.MAX_VALUE)) {
            val messages = Messages()
            // No payload: reading it before checking the length would throw EOF instead.
            reader(Buffer().header(opcode, size), messages).use {
                assertThrows(ProtocolException::class.java) { it.processNextFrame() }
            }
            assertTrue(messages.sizes.isEmpty())
        }
    }

    @Test fun fragmentsShareBudgetAcrossInterleavedControlFrames() {
        for (opcode in listOf(1, 2)) for (compressed in listOf(false, true)) {
            val wire = Buffer().header(opcode, limit.toLong(), final = false, compressed = compressed)
                .write(ByteArray(limit))
                .header(9, 0)
                .header(0, 1) // No continuation payload should be read.
            val messages = Messages()
            reader(wire, messages).use {
                assertThrows(ProtocolException::class.java) { it.processNextFrame() }
            }
            assertEquals(1, messages.pings)
            assertTrue(messages.sizes.isEmpty())
        }
    }

    @Test fun exactLimitFragmentsAndNextMessageAreAccepted() {
        for (opcode in listOf(1, 2)) {
            val wire = Buffer().header(opcode, 8192, final = false).write(ByteArray(8192) { 65 })
                .header(0, 8192).write(ByteArray(8192) { 65 })
                .header(opcode, 1).writeByte(65)
            val messages = Messages()
            reader(wire, messages).use { it.processNextFrame(); it.processNextFrame() }
            assertEquals(listOf(limit, 1), messages.sizes)
        }
    }

    @Test fun compressedBombsNeverReachEitherMessageCallback() {
        for (opcode in listOf(1, 2)) {
            val deflater = Deflater(9, true)
            try {
                val payload = compressed(deflater, limit * 64)
                assertTrue(payload.size < limit)
                val wire = Buffer().header(opcode, payload.size.toLong(), compressed = true).write(payload)
                val messages = Messages()
                reader(wire, messages).use {
                    assertThrows(ProtocolException::class.java) { it.processNextFrame() }
                }
                assertTrue(messages.sizes.isEmpty())
            } finally { deflater.end() }
        }
    }

    @Test fun fragmentedCompressedMessagesAtLimitReachBothCallbacks() {
        for (opcode in listOf(1, 2)) {
            val deflater = Deflater(9, true)
            try {
                val wire = Buffer()
                repeat(2) {
                    val payload = compressed(deflater, limit)
                    val half = payload.size / 2
                    wire.header(opcode, half.toLong(), final = false, compressed = true)
                        .write(payload, 0, half)
                        .header(9, 0)
                        .header(0, (payload.size - half).toLong())
                        .write(payload, half, payload.size - half)
                }
                val messages = Messages()
                reader(wire, messages).use { it.processNextFrame(); it.processNextFrame() }
                assertEquals(listOf(limit, limit), messages.sizes)
                assertEquals(2, messages.pings)
            } finally { deflater.end() }
        }
    }

    @Test fun inflationRejectsOneByteOverWithoutGrowingOutputPastLimit() {
        val deflater = Deflater(9, true)
        try {
            val output = Buffer().write(compressed(deflater, limit + 1))
            MessageInflater(false).use {
                assertThrows(ProtocolException::class.java) { it.inflate(output) }
            }
            assertEquals(limit.toLong(), output.size)
        } finally { deflater.end() }
    }

    @Test fun exactInflatedLimitWorksWithAndWithoutContextTakeover() {
        for (reset in listOf(false, true)) {
            val deflater = Deflater(9, true)
            try {
                MessageInflater(reset).use { inflater ->
                    repeat(2) {
                        if (reset) deflater.reset()
                        val output = Buffer().write(compressed(deflater, limit))
                        inflater.inflate(output)
                        assertArrayEquals(ByteArray(limit) { 65 }, output.readByteArray())
                    }
                }
            } finally { deflater.end() }
        }
    }

    @Test fun upstreamRealWebSocketUsesBoundedReaderAndFailsConnection() {
        var failure: Throwable? = null
        var delivered = false
        val socket = RealWebSocket(TaskRunner.INSTANCE, Request.Builder().url("http://localhost/").build(),
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, text: String) { delivered = true }
                override fun onFailure(webSocket: WebSocket, t: Throwable, response: okhttp3.Response?) { failure = t }
            }, Random(0), 0, WebSocketExtensions(), 1024)
        val wire = Buffer().header(1, limit + 1L)
        val streams = object : RealWebSocket.Streams(true, wire, Buffer()) {
            override fun close() {}
        }
        try {
            socket.initReaderAndWriter("receive-limit-test", streams)
            assertFalse(socket.processNextFrame())
            assertTrue(failure is ProtocolException)
            assertFalse(delivered)
        } finally { socket.tearDown() }
    }
}
