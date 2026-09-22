import Foundation

struct CommandQueue {
    private var pending: [Command] = []
    var isEmpty: Bool { pending.isEmpty }
    var count: Int { pending.count }

    @discardableResult
    mutating func append(_ command: Command) -> Bool {
        guard writeCount(command) != nil else { removeAll(); return false }
        if case .move(let dx, let dy) = command,
           case .move(let oldX, let oldY) = pending.last,
           (oldX + dx).isFinite, (oldY + dy).isFinite {
            pending[pending.count - 1] = .move(dx: oldX + dx, dy: oldY + dy)
        } else if case .scroll(let dx, let dy) = command,
                  case .scroll(let oldX, let oldY) = pending.last,
                  (oldX + dx).isFinite, (oldY + dy).isFinite {
            pending[pending.count - 1] = .scroll(dx: oldX + dx, dy: oldY + dy)
        } else {
            pending.append(command)
        }
        var writes = 0
        for command in pending {
            guard let count = writeCount(command) else { removeAll(); return false }
            writes += count
        }
        guard writes <= 64 else { removeAll(); return false }
        return true
    }

    private func writeCount(_ command: Command) -> Int? {
        switch command {
        case .move(let x, let y), .scroll(let x, let y):
            guard x.isFinite, y.isFinite, abs(x) <= 25_600, abs(y) <= 25_600 else { return nil }
            return max(1, Int(ceil(max(abs(x), abs(y)) / 400)))
        default: return 1
        }
    }

    mutating func popFirst() -> Command? {
        guard !pending.isEmpty else { return nil }
        var command = pending.removeFirst()
        switch command {
        case .move(let x, let y), .scroll(let x, let y):
            let dx = min(400, max(-400, x))
            let dy = min(400, max(-400, y))
            if case .move = command {
                command = .move(dx: dx, dy: dy)
                if x != dx || y != dy { pending.insert(.move(dx: x - dx, dy: y - dy), at: 0) }
            } else {
                command = .scroll(dx: dx, dy: dy)
                if x != dx || y != dy { pending.insert(.scroll(dx: x - dx, dy: y - dy), at: 0) }
            }
        default: break
        }
        return command
    }

    mutating func removeAll() { pending.removeAll() }
}
