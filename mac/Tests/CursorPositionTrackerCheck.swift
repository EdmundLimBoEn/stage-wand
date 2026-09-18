import Foundation

@main
struct CursorPositionTrackerCheck {
    static func main() {
        let now = ContinuousClock.now
        let origin = CGPoint(x: 100, y: 100)
        var tracker = CursorPositionTracker()
        precondition(tracker.reconcile(origin, now: now) == origin)
        for index in 1...100 {
            let current = tracker.reconcile(origin, now: now)
            precondition(current.x == 100 + Double(index - 1))
            tracker.didPost(CGPoint(x: current.x + 1, y: current.y), now: now)
        }
        precondition(tracker.reconcile(CGPoint(x: 140, y: 100), now: now).x == 200)
        precondition(tracker.reconcile(CGPoint(x: 180, y: 100), now: now).x == 200)
        precondition(tracker.reconcile(CGPoint(x: 200, y: 100), now: now).x == 200)
        tracker.didPost(CGPoint(x: 201, y: 100), now: now)
        let physical = CGPoint(x: 400, y: 300)
        precondition(tracker.reconcile(physical, now: now) == physical)
        tracker.didPost(CGPoint(x: 401, y: 300), now: now)
        precondition(tracker.reconcile(physical, now: now).x == 401)
        precondition(tracker.reconcile(physical, now: now.advanced(by: .milliseconds(251))) == physical)

        var edge = CursorPositionTracker()
        let clipped = CGPoint(x: 1919, y: 100)
        _ = edge.reconcile(clipped, now: now)
        for _ in 0..<20 { edge.didPost(clipped, now: now) }
        precondition(edge.reconcile(clipped, now: now) == clipped)
        edge.didPost(CGPoint(x: 1918, y: 100), now: now)
        precondition(edge.reconcile(clipped, now: now).x == 1918)

        var fractional = CursorPositionTracker()
        _ = fractional.reconcile(origin, now: now)
        for _ in 0..<10 {
            let current = fractional.reconcile(origin, now: now)
            fractional.didPost(CGPoint(x: current.x + 0.2, y: current.y), now: now)
        }
        precondition(abs(fractional.reconcile(origin, now: now).x - 102) < 0.000001)
        print("PASS: burst accumulation, intermediate catchup, physical handoff, timeout, clipped edge, fractional deltas")
    }
}
