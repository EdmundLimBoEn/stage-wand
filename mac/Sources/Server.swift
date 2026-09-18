import Foundation
import Network

@MainActor final class Server {
    private final class Peer {
        let connection: NWConnection
        var authed = false
        var lastPong = ContinuousClock.now
        var authTimeout: Task<Void, Never>?
        var heartbeat: Task<Void, Never>?
        var moves = 0
        init(_ connection: NWConnection) { self.connection = connection }
    }

    private let session: Session
    private let onCommand: @Sendable (Command) -> Void
    private let queue = DispatchQueue(label: "systems.edmundlim.stagewand.server")
    private var listener: NWListener?
    private var peers: [UUID: Peer] = [:]
    private var active: UUID?

    init(session: Session, onCommand: @escaping @Sendable (Command) -> Void) {
        self.session = session
        self.onCommand = onCommand
    }

    func start() throws -> UInt16 {
        if let port = listener?.port { return port.rawValue }
        var lastError: Error = NWError.posix(.EADDRINUSE)
        for port in UInt16(8787)...UInt16(8790) {
            let tcp = NWProtocolTCP.Options()
            tcp.noDelay = true
            let params = NWParameters(tls: nil, tcp: tcp)
            params.includePeerToPeer = true
            let websocket = NWProtocolWebSocket.Options()
            websocket.autoReplyPing = true
            websocket.maximumMessageSize = 16_384
            params.defaultProtocolStack.applicationProtocols.insert(websocket, at: 0)
            do {
                let candidate = try NWListener(using: params, on: NWEndpoint.Port(rawValue: port)!)
                candidate.service = NWListener.Service(name: Host.current().localizedName ?? "Mac", type: "_stagewand._tcp")
                let ready = DispatchSemaphore(value: 0)
                candidate.stateUpdateHandler = { state in
                    switch state {
                    case .ready, .failed: ready.signal()
                    default: break
                    }
                }
                candidate.newConnectionHandler = { [weak self] connection in
                    Task { @MainActor in self?.accept(connection) }
                }
                candidate.start(queue: queue)
                // Readiness must be known before returning the selected fallback port.
                let result = ready.wait(timeout: .now() + 2)
                if result == .success, case .ready = candidate.state {
                    listener = candidate
                    session.port = port
                    return port
                }
                if case .failed(let error) = candidate.state { lastError = error }
                else { lastError = NWError.posix(.ETIMEDOUT) }
                candidate.cancel()
            } catch { lastError = error }
        }
        throw lastError
    }

    func kick() {
        if let active { close(active, reason: "kicked") }
    }

    private func accept(_ connection: NWConnection) {
        let id = UUID()
        let peer = Peer(connection)
        peers[id] = peer
        connection.stateUpdateHandler = { [weak self] state in
            Task { @MainActor in
                switch state {
                case .failed, .cancelled: self?.close(id)
                default: break
                }
            }
        }
        connection.start(queue: queue)
        peer.authTimeout = Task { [weak self] in
            do { try await Task.sleep(for: .seconds(2)) } catch { return }
            guard let self, self.peers[id]?.authed == false else { return }
            self.close(id)
        }
        receive(id)
    }

    private func receive(_ id: UUID) {
        guard let peer = peers[id] else { return }
        peer.connection.receiveMessage { [weak self] data, context, _, error in
            Task { @MainActor in
                guard let self, let peer = self.peers[id] else { return }
                guard error == nil else { self.close(id); return }
                let metadata = context?.protocolMetadata(definition: NWProtocolWebSocket.definition) as? NWProtocolWebSocket.Metadata
                switch metadata?.opcode {
                case .close: self.close(id); return
                case .text:
                    guard let data, let command = try? JSONDecoder().decode(Command.self, from: data) else {
                        if !peer.authed { self.close(id); return }
                        self.receive(id)
                        return
                    }
                    self.handle(command, from: id)
                case .ping, .pong: break
                default:
                    if !peer.authed { self.close(id); return }
                }
                self.receive(id)
            }
        }
    }

    private func handle(_ command: Command, from id: UUID) {
        guard let peer = peers[id] else { return }
        if !peer.authed {
            guard case .auth(let code) = command else { close(id); return }
            guard code == session.code else { close(id, reason: "badauth"); return }
            if let active { close(active, reason: "displaced") }
            active = id
            peer.authed = true
            peer.authTimeout?.cancel()
            peer.lastPong = .now
            session.peer = "\(peer.connection.endpoint)"
            send(.status, to: id)
            peer.heartbeat = Task { [weak self] in
                while !Task.isCancelled {
                    do { try await Task.sleep(for: .seconds(3)) } catch { return }
                    guard let self, let peer = self.peers[id] else { return }
                    guard peer.lastPong.duration(to: .now) < .seconds(6) else {
                        self.close(id)
                        return
                    }
                    self.ping(id)
                }
            }
            return
        }
        guard active == id else { return }
        switch command {
        case .auth: return
        case .move(let dx, let dy):
            guard dx.isFinite, dy.isFinite, abs(dx) <= 400, abs(dy) <= 400 else { return }
            peer.moves += 1
            if peer.moves % 200 == 0 { print("MOVES \(peer.moves)") }
        case .scroll(let dx, let dy):
            guard dx.isFinite, dy.isFinite else { return }
        default: break
        }
        onCommand(command)
    }

    private func ping(_ id: UUID) {
        guard let peer = peers[id] else { return }
        let metadata = NWProtocolWebSocket.Metadata(opcode: .ping)
        metadata.setPongHandler(queue) { [weak self] error in
            Task { @MainActor in
                guard let self else { return }
                if error != nil { self.close(id) }
                else { self.peers[id]?.lastPong = .now }
            }
        }
        let context = NWConnection.ContentContext(identifier: "ping", metadata: [metadata])
        peer.connection.send(content: Data(), contentContext: context, isComplete: true, completion: .contentProcessed { [weak self] error in
            if error != nil { Task { @MainActor in self?.close(id) } }
        })
    }

    private func send(_ reply: Reply, to id: UUID) {
        guard let peer = peers[id], let data = try? JSONEncoder().encode(reply) else { return }
        let context = NWConnection.ContentContext(identifier: "reply", metadata: [NWProtocolWebSocket.Metadata(opcode: .text)])
        peer.connection.send(content: data, contentContext: context, isComplete: true, completion: .contentProcessed { [weak self] error in
            if error != nil { Task { @MainActor in self?.close(id) } }
        })
    }

    private func close(_ id: UUID, reason: String? = nil) {
        guard let peer = peers.removeValue(forKey: id) else { return }
        peer.authTimeout?.cancel()
        peer.heartbeat?.cancel()
        if active == id { active = nil; session.peer = nil }
        guard let reason, let data = try? JSONEncoder().encode(Reply.bye(reason: reason)) else {
            peer.connection.cancel()
            return
        }
        let connection = peer.connection
        let context = NWConnection.ContentContext(identifier: "bye", metadata: [NWProtocolWebSocket.Metadata(opcode: .text)])
        connection.send(content: data, contentContext: context, isComplete: true, completion: .contentProcessed { _ in
            let metadata = NWProtocolWebSocket.Metadata(opcode: .close)
            metadata.closeCode = .protocolCode(.normalClosure)
            let close = NWConnection.ContentContext(identifier: "close", metadata: [metadata])
            connection.send(content: nil, contentContext: close, isComplete: true, completion: .contentProcessed { _ in connection.cancel() })
        })
        Task {
            try? await Task.sleep(for: .seconds(1))
            connection.cancel()
        }
    }
}
