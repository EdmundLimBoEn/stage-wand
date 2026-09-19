package systems.edmundlim.stagewand.protocol

import kotlin.math.hypot
import kotlin.math.max
import kotlin.math.min

class SquareUnlock {
    var progress: Double = 0.0
        private set
    var failed: Boolean = false
        private set
    private var startedAt: Double? = null
    private var previous: Pair<Double, Double>? = null
    private var distance = 0.0
    private var backtracking = 0.0

    fun reset() {
        progress = 0.0
        failed = false
        startedAt = null
        previous = null
        distance = 0.0
        backtracking = 0.0
    }

    fun add(x: Double, y: Double, at: Double) {
        if (failed || !x.isFinite() || !y.isFinite() || !at.isFinite()) {
            failed = true
            return
        }
        val start = startedAt
        if (start == null) {
            if (hypot(x, y) > 0.10) {
                failed = true
                return
            }
            startedAt = at
            previous = x to y
            return
        }
        if (at < start || at - start > 12) {
            failed = true
            return
        }
        val clampedX = min(1.0, max(0.0, x))
        val clampedY = min(1.0, max(0.0, y))
        val candidates = listOf(
            clampedX to hypot(x - clampedX, y),
            (1 + clampedY) to hypot(x - 1, y - clampedY),
            (3 - clampedX) to hypot(x - clampedX, y - 1),
            (4 - clampedY) to hypot(x, y - clampedY)
        )
        val position = candidates
            .filter { it.second <= 0.10 && it.first >= progress - 0.12 && it.first <= progress + 0.35 }
            .minByOrNull { it.second }
            ?.first
        if (position == null) {
            failed = true
            return
        }
        backtracking += max(0.0, progress - position)
        previous?.let { (px, py) -> distance += hypot(x - px, y - py) }
        if (distance > 5.5 || backtracking > 0.5) {
            failed = true
            return
        }
        progress = max(progress, position)
        previous = x to y
    }

    fun finish(x: Double, y: Double, at: Double): Boolean {
        add(x, y, at)
        val start = startedAt
        return !failed && start != null && progress >= 3.90 && hypot(x, y) <= 0.10 &&
            distance >= 3.5 && at - start >= 0.85
    }
}
