import Foundation

struct AuthenticationLimiter {
    private var failures: [TimeInterval] = []

    mutating func authorize(_ candidate: String, expected: String,
                            now: TimeInterval = ProcessInfo.processInfo.systemUptime) -> Bool {
        failures.removeAll { now - $0 >= 30 }
        guard failures.count < 5 else { return false }
        guard candidate == expected else {
            failures.append(now)
            return false
        }
        reset()
        return true
    }

    mutating func reset() { failures.removeAll() }
}
