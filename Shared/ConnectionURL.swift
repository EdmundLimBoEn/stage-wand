import Foundation

enum ConnectionURL {
    static func parse(_ input: String) -> URL? {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed.rangeOfCharacter(from: .whitespacesAndNewlines) == nil else { return nil }
        let value = trimmed.contains("://") ? trimmed : "ws://\(trimmed)"
        guard var parts = URLComponents(string: value),
              let scheme = parts.scheme?.lowercased(),
              ["ws", "wss", "http", "https"].contains(scheme),
              let hostname = parts.host, !hostname.isEmpty,
              parts.user == nil, parts.password == nil, parts.fragment == nil else { return nil }
        // URLComponents treats an explicitly empty port as absent.
        let authority = value.components(separatedBy: "://")[1].prefix { $0 != "/" && $0 != "?" && $0 != "#" }
        guard !authority.hasSuffix(":"), !hostname.contains("\\") else { return nil }
        let secure = scheme == "https" || scheme == "wss"
        parts.scheme = secure ? "wss" : "ws"
        if parts.port == nil { parts.port = secure ? 443 : 8787 }
        guard let port = parts.port, (1...65535).contains(port) else { return nil }
        if parts.path.isEmpty { parts.path = "/" }
        return parts.url
    }

    static func pairingDestination(from url: URL) -> URL? {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false),
              parts.scheme?.lowercased() == "stagewand", parts.host?.lowercased() == "connect",
              parts.path.isEmpty || parts.path == "/",
              parts.user == nil, parts.password == nil, parts.port == nil, parts.fragment == nil,
              let items = parts.queryItems, items.count == 1,
              items[0].name == "url", let destination = items[0].value else { return nil }
        return parse(destination)
    }
}
