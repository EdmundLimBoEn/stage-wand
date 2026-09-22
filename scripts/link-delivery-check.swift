import Foundation

@MainActor
final class Session {
    let code = "4567"
    var port: UInt16 = 8787
    var peer: String?
    private var authentication = AuthenticationLimiter()
    private let ownership = PeerOwnership()

    func authorize(_ candidate: String) -> Bool {
        authentication.authorize(candidate, expected: code)
    }

    func claimPeer(id: UUID, name: String, onDisplaced: @escaping @MainActor () -> Void) {
        ownership.claim(id: id, onDisplaced: onDisplaced)
        peer = name
    }

    func releasePeer(id: UUID) {
        if ownership.release(id: id) { peer = nil }
    }

    func isActivePeer(id: UUID) -> Bool { ownership.contains(id: id) }
}

@MainActor
final class Recorder {
    private var storage: [Command] = []
    func add(_ command: Command) { storage.append(command) }
    func read() -> [Command] { storage }
}

@main
struct LinkDeliveryCheck {
    @MainActor
    static func main() async throws {
        let session = Session()
        let recorder = Recorder()
        let server = Server(session: session) { recorder.add($0) }
        let port = try server.start()
        let defaults = UserDefaults.standard
        defaults.set(session.code, forKey: "pairingCode")
        defaults.set("127.0.0.1:\(port)", forKey: "manualHost")
        defer { defaults.removeObject(forKey: "pairingCode"); defaults.removeObject(forKey: "manualHost") }
        let link = Link()
        for _ in 0..<10 { link.send(.key(.right)) }
        link.start()
        for _ in 0..<100 {
            if link.state == .authed { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        precondition(link.state == .authed && link.route == "Direct")
        for _ in 0..<1000 { link.send(.move(dx: 1, dy: -2)) }
        link.send(.click(.left))
        link.send(.move(dx: 12, dy: 34))
        link.send(.scroll(dx: 5, dy: 6))
        for _ in 0..<100 {
            if recorder.read().count >= 16 { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        let commands = recorder.read()
        precondition(commands.count == 16, "Missing or extra commands: \(commands.count)")
        for command in commands.prefix(8) {
            guard case .key(.right) = command else { fatalError("Buffered keys reordered") }
        }
        var sumX = 0.0, sumY = 0.0
        for command in commands[8..<13] {
            guard case .move(let x, let y) = command else { fatalError("Movement crossed click") }
            precondition(abs(x) <= 400 && abs(y) <= 400)
            sumX += x; sumY += y
        }
        precondition(sumX == 1000 && sumY == -2000)
        guard case .click(.left) = commands[13], case .move(let x, let y) = commands[14],
              case .scroll(let sx, let sy) = commands[15] else { fatalError("Barriers reordered") }
        precondition(x == 12 && y == 34 && sx == 5 && sy == 6)

        let replacement = LocalSocket(url: URL(string: "ws://127.0.0.1:\(port)")!)
        try await replacement.send(JSONEncoder().encode(Command.auth(code: session.code)))
        guard case .status = try JSONDecoder().decode(Reply.self, from: await replacement.receive()) else {
            fatalError("Replacement controller failed to authenticate")
        }
        try await Task.sleep(for: .milliseconds(2200))
        precondition(link.state == .disconnected, "Displaced controller must not reconnect and fight the replacement")
        link.send(.key(.left))
        defaults.set("9999", forKey: "pairingCode")
        link.start()
        for _ in 0..<100 {
            if link.state == .enterCode { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        precondition(link.state == .enterCode)
        replacement.cancel()
        defaults.set(session.code, forKey: "pairingCode")
        link.start()
        for _ in 0..<100 {
            if link.state == .authed { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        precondition(link.state == .authed)
        try await Task.sleep(for: .milliseconds(100))
        precondition(recorder.read().count == commands.count, "Changing credentials must discard buffered input")
        withExtendedLifetime(server) {}
        print("Link delivery checks passed: native direct auth, bounded buffered keys, exact movement and ordering, displaced-client reconnect suppression, credential-change input clearing")
    }
}
