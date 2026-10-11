import Foundation

@main
enum SessionSecurityCheck {
    @MainActor
    static func main() {
        var limiter = AuthenticationLimiter()
        for attempt in 0..<5 {
            precondition(!limiter.authorize("0000", expected: "1234", now: Double(attempt)))
        }
        precondition(!limiter.authorize("1234", expected: "1234", now: 5), "Lockout must include valid codes")
        precondition(!limiter.authorize("0000", expected: "1234", now: 29))
        precondition(limiter.authorize("1234", expected: "1234", now: 30), "Blocked attempts must not extend the window")
        for attempt in 0..<4 {
            precondition(!limiter.authorize("0000", expected: "1234", now: Double(31 + attempt)))
        }
        precondition(limiter.authorize("1234", expected: "1234", now: 35))
        for attempt in 0..<5 {
            precondition(!limiter.authorize("0000", expected: "1234", now: Double(36 + attempt)))
        }
        limiter.reset()
        precondition(limiter.authorize("5678", expected: "5678", now: 41), "Rotated codes clear the lockout")

        let ownership = PeerOwnership()
        let lan = UUID(), bluetooth = UUID()
        var lanDisplaced = 0, bluetoothDisplaced = 0
        ownership.claim(id: lan) {
            lanDisplaced += 1
            precondition(!ownership.release(id: lan), "Old transport close must not release the new transport")
        }
        ownership.claim(id: bluetooth) { bluetoothDisplaced += 1 }
        precondition(lanDisplaced == 1 && ownership.contains(id: bluetooth))
        precondition(!ownership.contains(id: lan))
        ownership.claim(id: bluetooth) { bluetoothDisplaced += 1 }
        precondition(bluetoothDisplaced == 0, "Reauthenticating the active transport must not displace itself")
        ownership.claim(id: lan) {}
        precondition(bluetoothDisplaced == 1 && ownership.contains(id: lan))
        precondition(!ownership.release(id: bluetooth))
        precondition(ownership.release(id: lan) && !ownership.contains(id: lan))
        print("Session security checks passed: bounded shared authentication throttle, expiry, resets, exclusive ownership, stale disconnect protection")
    }
}
