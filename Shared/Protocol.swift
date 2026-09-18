import Foundation

enum Button: String, Codable, Sendable { case left, right }
enum Key: String, Codable, Sendable { case left, right, esc }
enum Chord: String, Codable, Sendable { case spaceLeft, spaceRight, missionControl }
enum Command: Equatable, Sendable, Codable {
    case auth(code: String), move(dx: Double, dy: Double), click(Button), scroll(dx: Double, dy: Double), key(Key), chord(Chord)
    private enum Fields: String, CodingKey { case t, code, dx, dy, b, k }
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Fields.self)
        switch try c.decode(String.self, forKey: .t) {
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
        switch try c.decode(String.self, forKey: .t) {
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
