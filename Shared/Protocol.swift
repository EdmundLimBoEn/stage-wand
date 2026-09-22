import Foundation

enum Button: String, Codable, Sendable { case left, right }
enum Key: String, Codable, Sendable { case left, right, esc }
enum Chord: String, Codable, Sendable { case spaceLeft, spaceRight, missionControl }
enum Command: Equatable, Sendable, Codable {
    case auth(code: String), move(dx: Double, dy: Double), click(Button), scroll(dx: Double, dy: Double), key(Key), chord(Chord)
    private enum Fields: String, CodingKey { case t, code, dx, dy, b, k }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Fields.self)
        let type = try c.decode(String.self, forKey: .t)
        let fields: [String]
        switch type {
        case "auth": fields = ["t", "code"]
        case "move", "scroll": fields = ["t", "dx", "dy"]
        case "click": fields = ["t", "b"]
        case "key", "chord": fields = ["t", "k"]
        default: throw DecodingError.dataCorruptedError(forKey: .t, in: c, debugDescription: "Unknown command")
        }
        try requireFields(fields, from: decoder)
        switch type {
        case "auth": self = .auth(code: try c.decode(String.self, forKey: .code))
        case "move", "scroll":
            let x = try c.decode(Double.self, forKey: .dx), y = try c.decode(Double.self, forKey: .dy)
            guard x.isFinite && y.isFinite else { throw DecodingError.dataCorruptedError(forKey: .dx, in: c, debugDescription: "Nonfinite delta") }
            self = try c.decode(String.self, forKey: .t) == "move" ? .move(dx: x, dy: y) : .scroll(dx: x, dy: y)
        case "click": self = .click(try c.decode(Button.self, forKey: .b))
        case "key": self = .key(try c.decode(Key.self, forKey: .k))
        case "chord": self = .chord(try c.decode(Chord.self, forKey: .k))
        default: throw DecodingError.dataCorruptedError(forKey: .t, in: c, debugDescription: "Unknown command")
        }
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: Fields.self)
        switch self {
        case .auth(let code): try c.encode("auth", forKey: .t); try c.encode(code, forKey: .code)
        case .move(let x, let y), .scroll(let x, let y):
            guard x.isFinite && y.isFinite else { throw EncodingError.invalidValue(self, .init(codingPath: encoder.codingPath, debugDescription: "Nonfinite delta")) }
            if case .move = self { try c.encode("move", forKey: .t) } else { try c.encode("scroll", forKey: .t) }
            try c.encode(x, forKey: .dx); try c.encode(y, forKey: .dy)
        case .click(let b): try c.encode("click", forKey: .t); try c.encode(b, forKey: .b)
        case .key(let k): try c.encode("key", forKey: .t); try c.encode(k, forKey: .k)
        case .chord(let k): try c.encode("chord", forKey: .t); try c.encode(k, forKey: .k)
        }
    }
}
enum Reply: Equatable, Sendable, Codable {
    case status, bye(reason: String)
    private enum Fields: String, CodingKey { case t, reason }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Fields.self)
        let type = try c.decode(String.self, forKey: .t)
        let fields: [String]
        switch type {
        case "status": fields = ["t"]
        case "bye": fields = ["t", "reason"]
        default: throw DecodingError.dataCorruptedError(forKey: .t, in: c, debugDescription: "Unknown reply")
        }
        try requireFields(fields, from: decoder)
        switch type {
        case "status": self = .status
        case "bye": self = .bye(reason: try c.decode(String.self, forKey: .reason))
        default: throw DecodingError.dataCorruptedError(forKey: .t, in: c, debugDescription: "Unknown reply")
        }
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: Fields.self)
        switch self {
        case .status: try c.encode("status", forKey: .t)
        case .bye(let reason): try c.encode("bye", forKey: .t); try c.encode(reason, forKey: .reason)
        }
    }
}

private struct WireField: CodingKey {
    let stringValue: String
    var intValue: Int? { nil }
    init?(stringValue: String) { self.stringValue = stringValue }
    init?(intValue: Int) { return nil }
}

private func requireFields(_ expected: [String], from decoder: Decoder) throws {
    let fields = try decoder.container(keyedBy: WireField.self)
    guard Set(fields.allKeys.map(\.stringValue)) == Set(expected) else {
        throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "Unexpected frame fields"))
    }
}

enum WireProtocol {
    static func decodeCommand(_ data: Data) throws -> Command {
        try requireUniqueKeys(data)
        return try JSONDecoder().decode(Command.self, from: data)
    }

    static func decodeReply(_ data: Data) throws -> Reply {
        try requireUniqueKeys(data)
        return try JSONDecoder().decode(Reply.self, from: data)
    }

    // Codable containers collapse duplicate keys before init(from:) can inspect them.
    private static func requireUniqueKeys(_ data: Data) throws {
        guard String(data: data, encoding: .utf8) != nil else { throw invalidFrame() }
        let bytes = Array(data)
        var index = 0, depth = 0
        var expectingKey = true
        var keys = Set<String>()
        while index < bytes.count {
            let byte = bytes[index]
            if byte == 34 {
                let start = index
                index += 1
                while index < bytes.count {
                    if bytes[index] == 92 { index += 2; continue }
                    if bytes[index] == 34 { break }
                    index += 1
                }
                guard index < bytes.count else { throw invalidFrame() }
                if depth == 1 && expectingKey {
                    let key = try JSONDecoder().decode(String.self, from: Data(bytes[start...index]))
                    guard keys.insert(key).inserted else { throw invalidFrame() }
                }
            } else if byte == 123 || byte == 91 {
                depth += 1
            } else if byte == 125 || byte == 93 {
                if depth == 1 && expectingKey && !keys.isEmpty { throw invalidFrame() }
                depth -= 1
            } else if depth == 1 && byte == 44 {
                expectingKey = true
            } else if depth == 1 && byte == 58 {
                expectingKey = false
            }
            index += 1
        }
    }

    private static func invalidFrame() -> DecodingError {
        .dataCorrupted(.init(codingPath: [], debugDescription: "Invalid JSON frame or duplicate field"))
    }
}
