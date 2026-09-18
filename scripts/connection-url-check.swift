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
            (" [::1]:8787 ", "ws://[::1]:8787/")
        ]
        for (input, expected) in cases {
            precondition(ConnectionURL.parse(input)?.absoluteString == expected, "Failed \(input)")
        }
        for invalid in ["", "ws://", "ftp://example.com", "https://u:p@example.com", "wss://example.com/#fragment", "wss://example.com:0", "wss://example.com:65536", "wss://example.com:abc", "wss://example.com:", "wss://bad host/"] {
            precondition(ConnectionURL.parse(invalid) == nil, "Accepted \(invalid)")
        }
        var deepLink = URLComponents(string: "stagewand://connect")!
        deepLink.queryItems = [URLQueryItem(name: "url", value: "wss://tunnel.example/secret?token=a&b=c")]
        precondition(ConnectionURL.pairingDestination(from: deepLink.url!)?.absoluteString == "wss://tunnel.example:443/secret?token=a&b=c")
        for invalid in ["stagewand://other?url=wss://example.com", "stagewand://connect?url=wss://a&url=wss://b", "stagewand://connect?code=123456", "stagewand://connect?url=ftp://example.com"] {
            precondition(ConnectionURL.pairingDestination(from: URL(string: invalid)!) == nil)
        }
        print("Connection URL checks passed: 8 accepted forms, 10 rejected forms, nested deep-link query preservation and 4 invalid deep links")
    }
}
