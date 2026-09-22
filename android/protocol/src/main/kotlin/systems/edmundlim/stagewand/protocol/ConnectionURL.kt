package systems.edmundlim.stagewand.protocol

import java.net.URI
import java.net.URISyntaxException
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException

object ConnectionURL {
    fun parse(input: String): String? {
        val trimmed = input.trim()
        if (trimmed.isEmpty() || trimmed.any { it.isWhitespace() || it.isISOControl() }) return null
        val value = if ("://" in trimmed) trimmed else "ws://$trimmed"
        val parsed = uri(value) ?: return null
        val scheme = parsed.scheme?.lowercase() ?: return null
        if (scheme !in setOf("ws", "wss", "http", "https") || parsed.rawFragment != null) return null
        val authority = parsed.rawAuthority ?: return null
        val (host, portText) = splitAuthority(authority) ?: return null
        if (!validHost(host)) return null
        val secure = scheme == "https" || scheme == "wss"
        val port = if (portText == null) {
            if (secure) 443 else 8787
        } else {
            if (portText.isEmpty() || portText.any { it !in '0'..'9' }) return null
            portText.toIntOrNull() ?: return null
        }
        if (port !in 1..65535) return null
        val suffix = parsed.toASCIIString().substringAfter("://").drop(authority.length)
            .replace("[", "%5B").replace("]", "%5D")
        val pathAndQuery = if (suffix.isEmpty() || suffix.startsWith("?")) "/$suffix" else suffix
        return "${if (secure) "wss" else "ws"}://$host:$port$pathAndQuery"
    }

    fun pairingDestination(url: String): String? {
        val parsed = uri(url) ?: return null
        if (parsed.scheme?.lowercase() != "stagewand" || parsed.rawAuthority?.lowercase() != "connect") return null
        if (!parsed.rawPath.isNullOrEmpty() && parsed.rawPath != "/") return null
        if (parsed.rawFragment != null) return null
        val query = parsed.rawQuery ?: return null
        if (!query.startsWith("url=") || '&' in query) return null
        val destination = decodeQueryValue(query.drop(4)) ?: return null
        return parse(destination)
    }

    private fun uri(value: String): URI? = try {
        URI(value)
    } catch (_: URISyntaxException) {
        null
    }

    private fun splitAuthority(authority: String): Pair<String, String?>? {
        if (authority.startsWith("[")) {
            val end = authority.indexOf(']')
            if (end < 0) return null
            val suffix = authority.substring(end + 1)
            if (suffix.isNotEmpty() && !suffix.startsWith(":")) return null
            return authority.substring(0, end + 1) to if (suffix.isEmpty()) null else suffix.drop(1)
        }
        val pieces = authority.split(':')
        if (pieces.size > 2) return null
        return pieces[0] to pieces.getOrNull(1)
    }

    private fun validHost(host: String): Boolean {
        if (host.startsWith("[")) {
            if (!host.endsWith("]")) return false
            val address = host.drop(1).dropLast(1)
            if (':' !in address || '%' in address) return false
            if ('.' in address && !validIPv4(address.substringAfterLast(':'))) return false
            return uri("ws://[$address]")?.host != null
        }
        val hostname = host.removeSuffix(".")
        if (hostname.isEmpty() || hostname.length > 253) return false
        if ('.' in hostname && hostname.all { it in '0'..'9' || it == '.' }) return validIPv4(hostname)
        return hostname.split('.').all { label ->
            label.isNotEmpty() && label.length <= 63 && asciiAlphanumeric(label.first()) && asciiAlphanumeric(label.last()) &&
                label.all { asciiAlphanumeric(it) || it == '-' }
        }
    }

    private fun validIPv4(address: String): Boolean {
        val parts = address.split('.')
        return parts.size == 4 && parts.all { part ->
            part.isNotEmpty() && part.length <= 3 && (part.length == 1 || !part.startsWith('0')) &&
                part.all { it in '0'..'9' } && (part.toIntOrNull() ?: 256) <= 255
        }
    }

    private fun asciiAlphanumeric(char: Char): Boolean = char in '0'..'9' || char in 'A'..'Z' || char in 'a'..'z'

    private fun decodeQueryValue(value: String): String? {
        val bytes = java.io.ByteArrayOutputStream()
        var index = 0
        while (index < value.length) {
            if (value[index] == '%') {
                if (index + 2 >= value.length) return null
                val byte = value.substring(index + 1, index + 3).toIntOrNull(16) ?: return null
                bytes.write(byte)
                index += 3
            } else {
                val end = value.indexOf('%', index).let { if (it < 0) value.length else it }
                bytes.write(value.substring(index, end).toByteArray(Charsets.UTF_8))
                index = end
            }
        }
        return try {
            Charsets.UTF_8.newDecoder().decode(ByteBuffer.wrap(bytes.toByteArray())).toString()
        } catch (_: CharacterCodingException) {
            null
        }
    }
}
