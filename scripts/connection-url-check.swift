import Foundation

@main
struct ConnectionURLCheck {
    static func main() {
        let cases: [(String, String)] = [
            ("mac.local", "ws://mac.local:8787/"),
            ("192.168.1.2:9000", "ws://192.168.1.2:9000/"),
            ("http://mac.local", "ws://mac.local:8787/"),
            ("ws://mac.local:8787/hello?q=1", "ws://mac.local:8787/hello?q=1"),
            ("wss://tunnel.example/secret%2Fpath?a=1&b=%26", "wss://tunnel.example:443/secret%2Fpath?a=1&b=%26"),
            ("https://tunnel.example/secret?token=abc", "wss://tunnel.example:443/secret?token=abc"),
            ("https://tunnel.example:8443/", "wss://tunnel.example:8443/"),
            (" [::1]:8787 ", "ws://[::1]:8787/"),
            ("[::]", "ws://[::]:8787/"),
            ("[2001:db8:1:2:3:4:5:6]:9000", "ws://[2001:db8:1:2:3:4:5:6]:9000/"),
            ("[::ffff:192.168.1.2]", "ws://[::ffff:192.168.1.2]:8787/"),
            ("Mac.local.", "ws://Mac.local.:8787/"),
            ("WSS://Mac.local:00080/", "wss://Mac.local:80/"),
            ("wss://a/?token=a+b%2Bc", "wss://a:443/?token=a+b%2Bc"),
            ("wss://a/café?q=café", "wss://a:443/caf%C3%A9?q=caf%C3%A9"),
            ("wss://a/?q=[x]", "wss://a:443/?q=%5Bx%5D")
        ]
        for (input, expected) in cases {
            precondition(ConnectionURL.parse(input)?.absoluteString == expected, "Failed \(input): \(String(describing: ConnectionURL.parse(input)))")
        }
        let invalid = [
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
            "ws://[fe80::1%25en0]:9000/", "ws://[::1%en0]",
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
            "ws://a/\u{0}"
        ]
        for input in invalid {
            precondition(ConnectionURL.parse(input) == nil, "Accepted \(input)")
        }
        var deepLink = URLComponents(string: "stagewand://connect")!
        deepLink.queryItems = [URLQueryItem(name: "url", value: "wss://tunnel.example/secret?token=a&b=c")]
        precondition(ConnectionURL.pairingDestination(from: deepLink.url!)?.absoluteString == "wss://tunnel.example:443/secret?token=a&b=c")
        let deepLinks = [
            ("stagewand://connect?url=wss%3A%2F%2Fa%2F%3Ftoken%3Da%2Bb", "wss://a:443/?token=a+b"),
            ("stagewand://connect?url=wss://a/?token=a+b", "wss://a:443/?token=a+b"),
            ("stagewand://connect/?url=ws://[::1]:8787", "ws://[::1]:8787/"),
            ("STAGEWAND://CONNECT?url=ws://a", "ws://a:8787/")
        ]
        for (input, expected) in deepLinks {
            precondition(URL(string: input).flatMap { ConnectionURL.pairingDestination(from: $0) }?.absoluteString == expected, "Failed deep link \(input)")
        }
        let invalidDeepLinks = [
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
        ]
        for input in invalidDeepLinks {
            precondition(URL(string: input).flatMap { ConnectionURL.pairingDestination(from: $0) } == nil, "Accepted deep link \(input)")
        }
        print("Connection URL checks passed: \(cases.count) accepted URLs, \(invalid.count) rejected URLs, \(deepLinks.count + 1) accepted deep links, \(invalidDeepLinks.count) rejected deep links")
    }
}
