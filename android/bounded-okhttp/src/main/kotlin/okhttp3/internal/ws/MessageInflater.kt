/*
 * Copyright (C) 2020 Square, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package okhttp3.internal.ws
import java.io.Closeable
import java.io.IOException
import java.net.ProtocolException
import java.util.zip.Inflater
import okio.Buffer
import okio.InflaterSource

private const val OCTETS_TO_ADD_BEFORE_INFLATION = 0x0000ffff

class MessageInflater(
  private val noContextTakeover: Boolean
) : Closeable {
  private val deflatedBytes = Buffer()
  private val inflater = Inflater(true /* omit zlib header */)
  private val inflaterSource = InflaterSource(deflatedBytes, inflater)
  /** Inflates [buffer] in place as described in RFC 7692 section 7.2.2. */
  @Throws(IOException::class)
  fun inflate(buffer: Buffer) {
    require(deflatedBytes.size == 0L)

    if (noContextTakeover) {
      inflater.reset()
    }

    deflatedBytes.writeAll(buffer)
    deflatedBytes.writeInt(OCTETS_TO_ADD_BEFORE_INFLATION)

    val totalBytesToRead = inflater.bytesRead + deflatedBytes.size
    // We cannot read all, as the source does not close.
    // Instead, we ensure that all bytes from source have been processed by inflater.
    var inflated: Long
    do {
      val remaining = MAX_MESSAGE_BYTES - buffer.size
      if (remaining > 0L) {
        inflated = inflaterSource.readOrInflate(buffer, remaining)
      } else {
        // Probe in a separate one-byte buffer so the message never exceeds the limit.
        val overflow = Buffer()
        inflated = inflaterSource.readOrInflate(overflow, 1L)
        if (overflow.size != 0L) throw ProtocolException("Inflated message exceeds 16 KiB")
      }
      // Even after consuming the input, the inflater can have pending output.
    } while (inflater.bytesRead < totalBytesToRead || inflated > 0L)
  }

  @Throws(IOException::class)
  override fun close() = inflaterSource.close()
}
