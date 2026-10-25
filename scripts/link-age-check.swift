// Compile the production Link with deterministic transport doubles. No sockets or hardware.
import Foundation
import Network

@MainActor
final class SendProbe {
    static var latest: SendProbe?
    var commands: [Command] = []
    var cancelled = false
    var blocked: CheckedContinuation<Void, Error>?
    var reader: CheckedContinuation<Data, Error>?
    var authenticated = false
    var hold = true

    init() { Self.latest = self }
    func send(_ data: Data) async throws {
        let command = try JSONDecoder().decode(Command.self, from: data)
        if case .auth = command { return }
        commands.append(command)
        if hold {
            // Deliberately ignore task cancellation: the Link watchdog must cancel
            // the transport and clear its queue without waiting for this callback.
            try await withCheckedThrowingContinuation { blocked = $0 }
        }
    }
    func receive() async throws -> Data {
        if !authenticated {
            authenticated = true
            return Data(#"{"t":"status"}"#.utf8)
        }
        return try await withCheckedThrowingContinuation { reader = $0 }
    }
    func complete() { let pending = blocked; blocked = nil; pending?.resume() }
    func cancel() {
        cancelled = true
        let pending = reader; reader = nil
        pending?.resume(throwing: CancellationError())
    }
}

@MainActor
final class LocalSocket {
    let probe = SendProbe()
    init(url: URL, interface: NWInterface? = nil) {}
    func send(_ data: Data) async throws { try await probe.send(data) }
    func receive() async throws -> Data { try await probe.receive() }
    func cancel() { probe.cancel() }
}

// These names shadow Foundation's types only in this test executable.
@MainActor
final class URLSession {
    static let shared = URLSession()
    func webSocketTask(with url: URL) -> URLSessionWebSocketTask { URLSessionWebSocketTask() }
}
@MainActor
final class URLSessionWebSocketTask {
    enum Message { case data(Data), string(String) }
    enum CloseCode { case goingAway }
    var maximumMessageSize = 0
    let probe = SendProbe()
    func resume() {}
    func send(_ message: Message) async throws {
        switch message {
        case .data(let data): try await probe.send(data)
        case .string(let string): try await probe.send(Data(string.utf8))
        }
    }
    func receive() async throws -> Message { .data(try await probe.receive()) }
    func cancel(with code: CloseCode, reason: Data?) { probe.cancel() }
}

@MainActor
final class Discovery {
    var results: [(name: String, endpoint: NWEndpoint)] = []
    var onResults: (() -> Void)?
    var onDenied: (() -> Void)?
    var onFailure: (() -> Void)?
    func start() {}
    func stop() {}
    func resolve(_ endpoint: NWEndpoint, completion: @escaping @MainActor (URL?, NWInterface?) -> Void) {}
}
enum BluetoothError: Error { case unavailable, incompatible, tooLarge, disconnected }
@MainActor
final class BluetoothClient {
    var onName: ((String) -> Void)?
    func start() {}
    func send(_ data: Data, deadline: ContinuousClock.Instant?) async throws { fatalError("Unexpected Bluetooth") }
    func receive() async throws -> Data { fatalError("Unexpected Bluetooth") }
    func cancel() {}
}

@main
struct LinkAgeCheck {
    @MainActor
    static func waitUntil(_ predicate: () -> Bool) async throws {
        for _ in 0..<300 {
            if predicate() { return }
            try await Task.sleep(for: .milliseconds(10))
        }
        preconditionFailure("Timed out waiting for Link")
    }

    @MainActor
    static func start(_ address: String, buffered: Bool = false) async throws -> (Link, SendProbe) {
        let defaults = UserDefaults.standard
        defaults.set("1234", forKey: "pairingCode")
        defaults.set(address, forKey: "manualHost")
        defaults.set("wifi", forKey: "transport")
        let link = Link()
        if buffered {
            link.send(.key(.left))
            try await Task.sleep(for: .milliseconds(1400))
        }
        link.start()
        // Capture before yielding: settings notifications can reconnect older
        // Links and replace the global latest probe while authentication runs.
        let probe = SendProbe.latest!
        try await waitUntil { link.state == .authed }
        precondition(probe.authenticated && !probe.cancelled, "Expected this Link's authenticated transport")
        return (link, probe)
    }

    // Delay the main actor to exercise expiry before the sender can dispatch.
    static func stallMainActor() { Thread.sleep(forTimeInterval: 2.1) }

    @MainActor
    static func main() async throws {
        // Isolate test preferences from the user's app preferences.
        let defaults = UserDefaults.standard
        let keys = ["pairingCode", "manualHost", "transport"]
        let saved = keys.map { defaults.object(forKey: $0) }
        defer { for (key, value) in zip(keys, saved) { defaults.set(value, forKey: key) } }
        for address in ["ws://127.0.0.1:8787", "wss://example.invalid"] {
            // A progressing send must not refresh the backlog's original deadline.
            let (link, probe) = try await start(address)
            link.send(.click(.left))
            link.send(.key(.right))
            link.send(.key(.left))
            try await waitUntil { probe.blocked != nil }
            try await Task.sleep(for: .milliseconds(1200))
            probe.complete()
            try await waitUntil { probe.commands.count == 2 && probe.blocked != nil }
            try await Task.sleep(for: .milliseconds(1000))
            precondition(probe.cancelled, "Outstanding send must be cancelled at the original deadline")
            guard case .failed = link.state else { preconditionFailure("Expiry must fail the connection") }
            probe.complete() // A late successful callback must not drain the expired queue.
            try await Task.sleep(for: .milliseconds(30))
            precondition(probe.commands.count == 2, "Expired queued command was sent")

            // Reconnecting must not replay expired input; a drained backlog gets a new budget.
            link.start()
            let fresh = SendProbe.latest!
            try await waitUntil { link.state == .authed }
            precondition(fresh.authenticated && !fresh.cancelled, "Expected the reconnected transport")
            fresh.hold = false
            try await Task.sleep(for: .milliseconds(30))
            precondition(fresh.commands.isEmpty)
            link.send(.click(.right))
            try await waitUntil { fresh.commands.count == 1 }
            try await Task.sleep(for: .milliseconds(2200))
            precondition(!fresh.cancelled, "Drained backlog left an armed watchdog")
            fresh.hold = true
            link.send(.key(.right))
            try await waitUntil { fresh.blocked != nil }
            try await Task.sleep(for: .milliseconds(2200))
            precondition(fresh.cancelled, "New backlog did not arm its watchdog")
            fresh.complete()

            let (delayedLink, delayedProbe) = try await start(address)
            delayedLink.send(.click(.left))
            stallMainActor()
            try await Task.sleep(for: .milliseconds(50))
            precondition(delayedProbe.cancelled && delayedProbe.commands.isEmpty,
                         "Expired command reached the transport before the watchdog ran")

            // Enqueue must report rejection when it discovers expiry before the watchdog runs.
            let (expiredLink, expiredProbe) = try await start(address)
            precondition(expiredLink.send(.click(.left)))
            stallMainActor()
            precondition(!expiredLink.send(.key(.right)),
                         "Expired backlog must not confirm a discarded command")
            guard case .failed = expiredLink.state else { preconditionFailure("Expiry must fail the connection") }
            precondition(expiredProbe.cancelled)
            try await Task.sleep(for: .milliseconds(50))
            precondition(expiredProbe.commands.isEmpty, "Rejected backlog reached the transport")

            // Time spent buffered before authentication consumes the same two-second budget.
            let (bufferedLink, bufferedProbe) = try await start(address, buffered: true)
            try await waitUntil { bufferedProbe.blocked != nil }
            try await Task.sleep(for: .milliseconds(800))
            precondition(bufferedProbe.cancelled, "Authentication reset the command age")
            bufferedProbe.complete()
            withExtendedLifetime(bufferedLink) {}
        }
        print("Link command-age checks passed: WS/WSS expiry, progress, cancellation, queue discard, reconnect and buffered age")
    }
}
