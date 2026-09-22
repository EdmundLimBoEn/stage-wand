import Foundation

@main
struct CommandQueueCheck {
    static func main() {
        var queue = CommandQueue()
        for _ in 0..<1000 { queue.append(.move(dx: 1, dy: -2)) }
        precondition(queue.count == 1, "Slow sender retained individual frames")
        queue.append(.key(.right))
        queue.append(.move(dx: 10, dy: 20))
        queue.append(.click(.left))
        queue.append(.scroll(dx: 1, dy: 3))
        queue.append(.scroll(dx: 2, dy: 4))
        var x = 0.0, y = 0.0
        while let command = queue.popFirst() {
            if case .key = command { break }
            guard case .move(let dx, let dy) = command else { fatalError("Reordered barrier") }
            precondition(abs(dx) <= 400 && abs(dy) <= 400)
            x += dx; y += dy
        }
        precondition(x == 1000 && y == -2000, "Coalescing lost movement")
        guard case .move(let dx, let dy) = queue.popFirst() else { fatalError("Move crossed key barrier") }
        precondition(dx == 10 && dy == 20)
        guard case .click = queue.popFirst() else { fatalError("Click reordered") }
        guard case .scroll(let sx, let sy) = queue.popFirst() else { fatalError("Scroll crossed click") }
        precondition(sx == 3 && sy == 7 && queue.isEmpty)
        queue.append(.move(dx: 1, dy: 1))
        queue.removeAll()
        precondition(queue.popFirst() == nil)
        for _ in 0..<1000 { queue.append(.scroll(dx: 1, dy: -2)) }
        queue.append(.click(.left))
        x = 0; y = 0
        var pieces = 0
        while let command = queue.popFirst() {
            if case .click = command { break }
            guard case .scroll(let dx, let dy) = command else { fatalError("Scroll reordered") }
            precondition(abs(dx) <= 400 && abs(dy) <= 400)
            x += dx; y += dy; pieces += 1
        }
        precondition(x == 1000 && y == -2000 && pieces == 5 && queue.isEmpty, "Scroll splitting lost distance")
        for _ in 0..<64 { precondition(queue.append(.key(.right))) }
        precondition(!queue.append(.click(.left)) && queue.isEmpty)
        precondition(queue.append(.move(dx: 25_600, dy: 0)))
        precondition(!queue.append(.move(dx: 0.1, dy: 0)) && queue.isEmpty)
        precondition(!queue.append(.move(dx: 1e308, dy: 0)))
        precondition(!queue.append(.scroll(dx: 0, dy: .nan)))
        precondition(!queue.append(.scroll(dx: .infinity, dy: 0)) && queue.isEmpty)
        print("Command queue checks passed: movement/scroll coalescing, exact sums, legal splits, key/click barriers, reset")
    }
}
