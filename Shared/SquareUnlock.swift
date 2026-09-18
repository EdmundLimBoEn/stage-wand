import Foundation

/// A clockwise trace around a unit square, beginning at its top-left corner.
struct SquareUnlock {
    private(set) var progress: Double = 0
    private(set) var failed = false
    private var startedAt: TimeInterval?
    private var previous: (x: Double, y: Double)?
    private var distance: Double = 0
    private var backtracking: Double = 0

    mutating func reset() {
        self = SquareUnlock()
    }

    mutating func add(x: Double, y: Double, at time: TimeInterval) {
        guard !failed, x.isFinite, y.isFinite, time.isFinite else {
            failed = true
            return
        }
        guard let start = startedAt else {
            guard hypot(x, y) <= 0.10 else { failed = true; return }
            startedAt = time
            previous = (x, y)
            return
        }
        guard time >= start, time - start <= 12 else { failed = true; return }
        let clampedX = min(1, max(0, x))
        let clampedY = min(1, max(0, y))
        let candidates: [(Double, Double)] = [
            (clampedX, hypot(x - clampedX, y)),
            (1 + clampedY, hypot(x - 1, y - clampedY)),
            (3 - clampedX, hypot(x - clampedX, y - 1)),
            (4 - clampedY, hypot(x, y - clampedY))
        ]
        guard let position = candidates
            .filter({ $0.1 <= 0.10 && $0.0 >= progress - 0.12 && $0.0 <= progress + 0.35 })
            .min(by: { $0.1 < $1.1 })?.0 else { failed = true; return }
        backtracking += max(0, progress - position)
        if let previous { distance += hypot(x - previous.x, y - previous.y) }
        guard distance <= 5.5, backtracking <= 0.5 else { failed = true; return }
        progress = max(progress, position)
        previous = (x, y)
    }

    mutating func finish(x: Double, y: Double, at time: TimeInterval) -> Bool {
        add(x: x, y: y, at: time)
        guard !failed, let startedAt else { return false }
        return progress >= 3.90 && hypot(x, y) <= 0.10
            && distance >= 3.5 && time - startedAt >= 0.85
    }
}
