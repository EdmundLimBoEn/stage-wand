import Foundation

struct CommandQueue {
    private var pending: [Command] = []
    var isEmpty: Bool { pending.isEmpty }
    var count: Int { pending.count }

    mutating func append(_ command: Command) {
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
    }

    mutating func popFirst() -> Command? {
        guard !pending.isEmpty else { return nil }
        var command = pending.removeFirst()
        if case .move(let x, let y) = command {
            let dx = min(400, max(-400, x))
            let dy = min(400, max(-400, y))
            command = .move(dx: dx, dy: dy)
            if x != dx || y != dy { pending.insert(.move(dx: x - dx, dy: y - dy), at: 0) }
        }
        return command
    }

    mutating func removeAll() { pending.removeAll() }
}
