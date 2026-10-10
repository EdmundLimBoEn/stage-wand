package systems.edmundlim.stagewand.protocol

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class ConnectionURLRobustnessTest {
    @Test fun supportedAuthoritiesAndQueries() {
        val cases = listOf(
            "mac.local" to "ws://mac.local:8787/",
            "192.168.1.2:9000" to "ws://192.168.1.2:9000/",
            "http://mac.local" to "ws://mac.local:8787/",
            "ws://mac.local:8787/hello?q=1" to "ws://mac.local:8787/hello?q=1",
            "wss://tunnel.example/secret%2Fpath?a=1&b=%26" to "wss://tunnel.example:443/secret%2Fpath?a=1&b=%26",
            "https://tunnel.example/secret?token=abc" to "wss://tunnel.example:443/secret?token=abc",
            "https://tunnel.example:8443/" to "wss://tunnel.example:8443/",
            " [::1]:8787 " to "ws://[::1]:8787/",
            "[::]" to "ws://[::]:8787/",
            "[2001:db8:1:2:3:4:5:6]:9000" to "ws://[2001:db8:1:2:3:4:5:6]:9000/",
            "[::ffff:192.168.1.2]" to "ws://[::ffff:192.168.1.2]:8787/",
            "Mac.local." to "ws://Mac.local.:8787/",
            "WSS://Mac.local:00080/" to "wss://Mac.local:80/",
            "wss://a/?token=a+b%2Bc" to "wss://a:443/?token=a+b%2Bc",
            "wss://a/café?q=café" to "wss://a:443/caf%C3%A9?q=caf%C3%A9",
            "wss://a/?q=[x]" to "wss://a:443/?q=%5Bx%5D"
        )
        for ((input, expected) in cases) assertEquals(expected, ConnectionURL.parse(input), input)
    }

    @Test fun malformedAuthoritiesAndEscapesAreRejected() {
        val invalid = listOf(
            "",
            "ws://",
            "ftp://example.com",
            "https://u:p@example.com",
            "wss://example.com/#fragment",
            "wss://example.com:0",
            "wss://example.com:65536",
            "wss://example.com:abc",
            "wss://example.com:",
            "wss://bad host/",
            "ws://[]:8787",
            "ws://[::1]garbage",
            "ws://[:::]:8787",
            "ws://[a]:8787",
            "ws://[1:2:3:4:5:6:7]",
            "ws://[1:2:3:4:5:6:7:8:9]",
            "ws://[::ffff:192.168.001.2]",
            "ws://[::ffff:256.1.1.1]",
            "ws://[fe80::1%25en0]:9000/",
            "ws://[::1%en0]",
            "ws://[::1%25]",
            "ws://[::1%25en0%25x]",
            "ws://::1",
            "ws://127.0.0.1:1:2",
            "ws://a:999999999999999999999999",
            "ws://a:+80",
            "ws://a:-80",
            "ws://a:８０",
            "ws://256.1.1.1",
            "ws://127.1",
            "ws://127.00.0.1",
            "ws://.local",
            "ws://a..local",
            "ws://-a.local",
            "ws://a-.local",
            "ws://a_b.local",
            "ws://foo%2Ecom",
            "ws://foo\\bar",
            "ws://a/%",
            "ws://a/%xy",
            "ws://a/[x]",
            "ws://a/a|b",
            "ws://a/\u0000"
        )
        for (input in invalid) assertNull(ConnectionURL.parse(input), input)
    }

    @Test fun deepLinksPreservePlusSigns() {
        val cases = listOf(
            "stagewand://connect?url=wss%3A%2F%2Fa%2F%3Ftoken%3Da%2Bb" to "wss://a:443/?token=a+b",
            "stagewand://connect?url=wss://a/?token=a+b" to "wss://a:443/?token=a+b",
            "stagewand://connect/?url=ws://[::1]:8787" to "ws://[::1]:8787/",
            "STAGEWAND://CONNECT?url=ws://a" to "ws://a:8787/"
        )
        for ((input, expected) in cases) assertEquals(expected, ConnectionURL.pairingDestination(input), input)
    }

    @Test fun malformedDeepLinksNeverThrow() {
        val invalid = listOf(
            "stagewand://other?url=wss://example.com",
            "stagewand://connect?url=wss://a&url=wss://b",
            "stagewand://connect?code=123456",
            "stagewand://connect?url=ftp://example.com",
            "stagewand://connect?url=%",
            "stagewand://connect?url=%GG",
            "stagewand://connect?url=wss://a/%",
            "stagewand://connect?url=wss://a/%C3%28",
            "stagewand://connect:?url=ws://a",
            "stagewand://connect:0?url=ws://a",
            "stagewand://user@connect?url=ws://a",
            "stagewand://%63onnect?url=ws://a",
            "stagewand://connect/extra?url=ws://a",
            "stagewand://connect/%2F?url=ws://a",
            "stagewand://connect?url=ws://a&",
            "stagewand://connect?url=ws://a#fragment",
            "stagewand://connect?url=",
            "stagewand://[broken?url=ws://a"
        )
        for (input in invalid) assertNull(ConnectionURL.pairingDestination(input), input)
    }
}
