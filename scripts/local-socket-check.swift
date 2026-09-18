import Foundation
import Network
import Darwin

// Minimal state for compiling the production Server without its menu UI.
@MainActor
final class Session {
    let code = "4567"
    var port: UInt16 = 8787
    var peer: String?
}

@main
struct LocalSocketCheck {
    @MainActor
    static func main() async throws {
        let watchdog = Task {
            try await Task.sleep(for: .seconds(18))
            fatalError("Local socket check timed out")
        }
        defer { watchdog.cancel() }
        let session = Session()
        let server = Server(session: session) { _ in }
        let port = try server.start()
        let discovery = Discovery()
        let endpoint = NWEndpoint.hostPort(host: "127.0.0.1", port: NWEndpoint.Port(rawValue: port)!)
        let resolved: (URL?, NWInterface?) = await withCheckedContinuation { continuation in
            discovery.resolve(endpoint) { url, interface in continuation.resume(returning: (url, interface)) }
        }
        guard let url = resolved.0 else { fatalError("Local endpoint not resolved") }
        let socket = LocalSocket(url: url, interface: resolved.1)
        try await socket.send(JSONEncoder().encode(Command.auth(code: session.code)))
        guard case .status = try JSONDecoder().decode(Reply.self, from: await socket.receive()) else {
            fatalError("Authentication failed")
        }
        let pendingReceive = Task { try await socket.receive() }
        // Production server pings every three seconds and expires after six without pong.
        try await Task.sleep(for: .seconds(7))
        precondition(session.peer != nil, "Automatic pong failed")
        try await socket.send(JSONEncoder().encode(Command.move(dx: 100, dy: -50)))
        pendingReceive.cancel()
        do { _ = try await pendingReceive.value; fatalError("Cancelled receive succeeded") }
        catch { }
        socket.cancel()

        let rejected = LocalSocket(url: url)
        try await rejected.send(JSONEncoder().encode(Command.auth(code: "9999")))
        guard case .bye(let reason) = try JSONDecoder().decode(Reply.self, from: await rejected.receive()), reason == "badauth" else {
            fatalError("Bad code not rejected")
        }
        rejected.cancel()
        withExtendedLifetime(server) {}
        print("Local socket checks passed: scoped endpoint resolution, live production WebSocket auth, automatic heartbeat pong, send, receive cancellation, bad-code reply")
    }
}
