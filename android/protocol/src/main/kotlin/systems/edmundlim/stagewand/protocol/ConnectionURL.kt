package systems.edmundlim.stagewand.protocol

object ConnectionURL {
    fun parse(input: String): String? {
        val trimmed = input.trim()
        if (trimmed.isEmpty() || trimmed.any { it.isWhitespace() }) return null
        val value = if ("://" in trimmed) trimmed else "ws://$trimmed"
        val schemeEnd = value.indexOf("://")
        if (schemeEnd <= 0) return null
        val scheme = value.substring(0, schemeEnd).lowercase()
        if (scheme !in setOf("ws", "wss", "http", "https")) return null
        val rest = value.substring(schemeEnd + 3)
        if (rest.isEmpty() || "@" in rest.substringBefore("/") ) {
            val beforeSlash = rest.substringBefore("/").substringBefore("?").substringBefore("#")
            if ("@" in beforeSlash) return null
        }
        if ("#" in rest) return null
        val authorityEnd = rest.indexOfFirst { it == '/' || it == '?' || it == '#' }.let { if (it < 0) rest.length else it }
        val authority = rest.substring(0, authorityEnd)
        if (authority.isEmpty() || authority.endsWith(":")) return null
        val host = hostOf(authority) ?: return null
        if (host.isEmpty() || "\\" in host) return null
        val portText = portText(authority)
        val secure = scheme == "https" || scheme == "wss"
        val port = if (portText == null) {
            if (secure) 443 else 8787
        } else {
            portText.toIntOrNull() ?: return null
        }
        if (port !in 1..65535) return null
        var pathAndQuery = rest.substring(authorityEnd)
        if (pathAndQuery.isEmpty() || pathAndQuery.startsWith("?")) {
            pathAndQuery = "/$pathAndQuery"
        }
        val outScheme = if (secure) "wss" else "ws"
        return "$outScheme://$host:$port$pathAndQuery"
    }

    fun pairingDestination(url: String): String? {
        val parsed = java.net.URI(url)
        if (parsed.scheme?.lowercase() != "stagewand") return null
        if (parsed.host?.lowercase() != "connect") return null
        if (parsed.path != null && parsed.path.isNotEmpty() && parsed.path != "/") return null
        if (parsed.userInfo != null || parsed.port != -1 || parsed.fragment != null) return null
        val query = parsed.rawQuery ?: return null
        val items = query.split("&").map { part ->
            val idx = part.indexOf("=")
            if (idx < 0) part to ""
            else part.substring(0, idx) to part.substring(idx + 1)
        }
        if (items.size != 1 || items[0].first != "url") return null
        val destination = java.net.URLDecoder.decode(items[0].second, Charsets.UTF_8)
        return parse(destination)
    }

    private fun hostOf(authority: String): String? {
        if (authority.startsWith("[")) {
            val end = authority.indexOf(']')
            if (end < 0) return null
            return authority.substring(0, end + 1)
        }
        return authority.substringBefore(":")
    }

    private fun portText(authority: String): String? {
        if (authority.startsWith("[")) {
            val end = authority.indexOf(']')
            if (end < 0) return null
            if (end + 1 >= authority.length) return null
            if (authority[end + 1] != ':') return null
            return authority.substring(end + 2)
        }
        val colon = authority.lastIndexOf(':')
        if (colon < 0) return null
        return authority.substring(colon + 1)
    }
}
