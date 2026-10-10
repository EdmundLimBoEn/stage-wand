import Foundation
#if canImport(Darwin)
import Darwin
#elseif canImport(Glibc)
import Glibc
#endif

enum ConnectionURL {
    static func parse(_ input: String) -> URL? {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              trimmed.rangeOfCharacter(from: .whitespacesAndNewlines.union(.controlCharacters)) == nil else { return nil }
        let value = trimmed.contains("://") ? trimmed : "ws://\(trimmed)"
        guard let separator = value.range(of: "://") else { return nil }
        let scheme = value[..<separator.lowerBound].lowercased()
        guard ["ws", "wss", "http", "https"].contains(scheme) else { return nil }
        let rest = value[separator.upperBound...]
        let authority = String(rest.prefix { $0 != "/" && $0 != "?" && $0 != "#" })
        guard let (host, portText) = splitAuthority(authority), validHost(host) else { return nil }
        let secure = scheme == "https" || scheme == "wss"
        let port: Int
        if let portText {
            guard !portText.isEmpty, portText.utf8.allSatisfy({ (48...57).contains($0) }),
                  let explicitPort = Int(portText), (1...65535).contains(explicitPort) else { return nil }
            port = explicitPort
        } else {
            port = secure ? 443 : 8787
        }
        var suffix = String(rest.dropFirst(authority.count))
        guard validSuffix(suffix) else { return nil }
        if suffix.isEmpty || suffix.hasPrefix("?") { suffix = "/" + suffix }
        return URL(string: "\(secure ? "wss" : "ws")://\(host):\(port)\(suffix)")
    }

    static func pairingDestination(from url: URL) -> URL? {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false),
              parts.scheme?.lowercased() == "stagewand",
              let separator = url.absoluteString.range(of: "://"),
              url.absoluteString[separator.upperBound...].prefix(while: { $0 != "/" && $0 != "?" && $0 != "#" }).lowercased() == "connect",
              parts.percentEncodedPath.isEmpty || parts.percentEncodedPath == "/",
              parts.fragment == nil, let query = parts.percentEncodedQuery,
              query.hasPrefix("url="), !query.contains("&"),
              let destination = String(query.dropFirst(4)).removingPercentEncoding else { return nil }
        return parse(destination)
    }

    private static func splitAuthority(_ authority: String) -> (String, String?)? {
        if authority.hasPrefix("[") {
            guard let end = authority.firstIndex(of: "]") else { return nil }
            let host = String(authority[...end])
            let suffix = authority[authority.index(after: end)...]
            guard suffix.isEmpty || suffix.hasPrefix(":") else { return nil }
            return (host, suffix.isEmpty ? nil : String(suffix.dropFirst()))
        }
        let pieces = authority.split(separator: ":", omittingEmptySubsequences: false)
        guard pieces.count <= 2 else { return nil }
        return (String(pieces[0]), pieces.count == 2 ? String(pieces[1]) : nil)
    }

    private static func validHost(_ host: String) -> Bool {
        if host.hasPrefix("[") {
            guard host.hasSuffix("]") else { return false }
            let address = String(host.dropFirst().dropLast())
            guard !address.contains("%") else { return false }
            if address.contains("."), !validIPv4(String(address.split(separator: ":").last ?? "")) { return false }
            var binary = in6_addr()
            return address.withCString { inet_pton(AF_INET6, $0, &binary) } == 1
        }
        let hostname = host.hasSuffix(".") ? String(host.dropLast()) : host
        guard !hostname.isEmpty, hostname.utf8.count <= 253 else { return false }
        if hostname.contains("."), hostname.utf8.allSatisfy({ (48...57).contains($0) || $0 == 46 }) {
            return validIPv4(hostname)
        }
        return hostname.split(separator: ".", omittingEmptySubsequences: false).allSatisfy { label in
            let bytes = Array(label.utf8)
            return !bytes.isEmpty && bytes.count <= 63 && asciiAlphanumeric(bytes[0]) && asciiAlphanumeric(bytes[bytes.count - 1])
                && bytes.allSatisfy { asciiAlphanumeric($0) || $0 == 45 }
        }
    }

    private static func validIPv4(_ address: String) -> Bool {
        let parts = address.split(separator: ".", omittingEmptySubsequences: false)
        return parts.count == 4 && parts.allSatisfy { part in
            !part.isEmpty && part.count <= 3 && (part.count == 1 || !part.hasPrefix("0"))
                && part.utf8.allSatisfy { (48...57).contains($0) } && (Int(part) ?? 256) <= 255
        }
    }

    private static func asciiAlphanumeric(_ byte: UInt8) -> Bool {
        (48...57).contains(byte) || (65...90).contains(byte) || (97...122).contains(byte)
    }

    private static func validSuffix(_ suffix: String) -> Bool {
        let bytes = Array(suffix.utf8)
        let punctuation = Array("-._~!$&'()*+,;=:@/?".utf8)
        var query = false
        var index = 0
        while index < bytes.count {
            let byte = bytes[index]
            if byte == 63 { query = true }
            if byte == 37 {
                guard index + 2 < bytes.count,
                      bytes[(index + 1)...(index + 2)].allSatisfy({ (48...57).contains($0) || (65...70).contains($0) || (97...102).contains($0) }) else { return false }
                index += 3
                continue
            }
            if byte < 128 && !asciiAlphanumeric(byte) && !punctuation.contains(byte)
                && !(query && (byte == 91 || byte == 93)) { return false }
            index += 1
        }
        return true
    }
}
