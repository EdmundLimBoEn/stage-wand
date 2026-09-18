import Foundation
import Combine

@MainActor
final class Link: ObservableObject {
    enum State: Equatable {
        case searching, connecting(String), enterCode, authed, disconnected, localNetworkDenied
    }
    @Published var state: State = .searching
    @Published var macName: String?
    private let discovery = Discovery()
    private var socket: URLSessionWebSocketTask?
    private var receiveTask: Task<Void, Never>?
    private var sendTask: Task<Void, Never>?
    private var ticker: Task<Void, Never>?
    private var retryTask: Task<Void, Never>?
    private var timeoutTask: Task<Void, Never>?
    private var defaultsObserver: AnyCancellable?
    private var generation = UUID()
    private var code = ""
    private var host = ""
    private var buffered: [Command] = []
    private var outgoing: [Command] = []
    private var move = (x: 0.0, y: 0.0)
    private var scroll = (x: 0.0, y: 0.0)

    func start() {
        discovery.onResults = { [weak self] in
            guard let self else { return }
            if self.state == .searching || self.state == .disconnected { self.connect() }
        }
        discovery.onDenied = { [weak self] in
            guard let self, self.host.isEmpty, self.state != .authed else { return }
            self.resetConnection()
            self.state = .localNetworkDenied
        }
        if defaultsObserver == nil {
            defaultsObserver = NotificationCenter.default.publisher(for: UserDefaults.didChangeNotification)
                .sink { [weak self] _ in
                    Task { @MainActor in self?.refreshSettings() }
                }
        }
        if ticker == nil {
            ticker = Task { @MainActor [weak self] in
                while !Task.isCancelled {
                    do { try await Task.sleep(for: .milliseconds(16.666667)) } catch { return }
                    self?.flushDeltas()
                }
            }
        }
        refreshSettings(force: true)
    }

    func send(_ command: Command) {
        switch command {
        case .auth: return
        case .move(let dx, let dy):
            guard state == .authed, dx.isFinite, dy.isFinite,
                  (move.x + dx).isFinite, (move.y + dy).isFinite else { return }
            move.x += dx
            move.y += dy
        case .scroll(let dx, let dy):
            guard state == .authed, dx.isFinite, dy.isFinite,
                  (scroll.x + dx).isFinite, (scroll.y + dy).isFinite else { return }
            scroll.x += dx
            scroll.y += dy
        case .key, .click, .chord:
            if state == .authed {
                flushDeltas()
                enqueue(command)
            } else {
                if buffered.count == 8 { buffered.removeFirst() }
                buffered.append(command)
            }
        }
    }

    private func refreshSettings(force: Bool = false) {
        let newCode = UserDefaults.standard.string(forKey: "pairingCode") ?? ""
        let newHost = (UserDefaults.standard.string(forKey: "manualHost") ?? "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard force || newCode != code || newHost != host else { return }
        code = newCode
        host = newHost
        resetConnection()
        state = code.isEmpty ? .enterCode : .searching
        if !code.isEmpty { connect() }
    }

    private func connect() {
        guard socket == nil else { return }
        guard !code.isEmpty else { state = .enterCode; return }
        retryTask?.cancel()
        retryTask = nil
        if !host.isEmpty {
            macName = host
            guard let url = Self.manualURL(host) else { disconnected(); return }
            open(url)
            return
        }
        discovery.start()
        guard let result = discovery.results.first else { state = .searching; return }
        macName = result.name
        state = .connecting(result.name)
        let id = generation
        armTimeout(id)
        discovery.resolve(result.endpoint) { [weak self] url in
            guard let self, self.generation == id else { return }
            if let url { self.open(url) } else { self.disconnected() }
        }
    }

    private static func manualURL(_ host: String) -> URL? {
        let value = host.contains("://") ? host : "ws://\(host)"
        guard var parts = URLComponents(string: value), parts.scheme == "ws",
              let hostname = parts.host, !hostname.isEmpty,
              parts.user == nil, parts.password == nil else { return nil }
        if parts.port == nil { parts.port = 8787 }
        guard let port = parts.port, (1...65535).contains(port) else { return nil }
        parts.path = "/"
        parts.query = nil
        parts.fragment = nil
        return parts.url
    }

    private func open(_ url: URL) {
        state = .connecting(macName ?? url.host ?? "Mac")
        let socket = URLSession.shared.webSocketTask(with: url)
        self.socket = socket
        let id = generation
        armTimeout(id)
        socket.resume()
        enqueue(.auth(code: code))
        receiveTask = Task { @MainActor [weak self] in
            do {
                while !Task.isCancelled {
                    let message = try await socket.receive()
                    guard let self, self.generation == id else { return }
                    let data: Data
                    switch message {
                    case .data(let value): data = value
                    case .string(let value): data = Data(value.utf8)
                    @unknown default: continue
                    }
                    let reply = try JSONDecoder().decode(Reply.self, from: data)
                    switch reply {
                    case .status:
                        self.timeoutTask?.cancel()
                        self.state = .authed
                        let pending = self.buffered
                        self.buffered.removeAll()
                        for command in pending { self.enqueue(command) }
                    case .bye(let reason):
                        self.resetConnection()
                        if reason == "badauth" { self.state = .enterCode }
                        else { self.disconnected() }
                        return
                    }
                }
            } catch {
                guard let self, self.generation == id else { return }
                self.disconnected()
            }
        }
    }

    private func enqueue(_ command: Command) {
        outgoing.append(command)
        guard sendTask == nil, let socket else { return }
        let id = generation
        sendTask = Task { @MainActor [weak self] in
            do {
                while let self, self.generation == id, !self.outgoing.isEmpty {
                    let command = self.outgoing.removeFirst()
                    let data = try JSONEncoder().encode(command)
                    try await socket.send(.string(String(decoding: data, as: UTF8.self)))
                }
                guard let self, self.generation == id else { return }
                self.sendTask = nil
            } catch {
                guard let self, self.generation == id else { return }
                self.disconnected()
            }
        }
    }

    private func flushDeltas() {
        guard state == .authed else { return }
        // Split large sums into legal frames without clamping away pointer distance.
        while move.x != 0 || move.y != 0 {
            let dx = min(400, max(-400, move.x))
            let dy = min(400, max(-400, move.y))
            move.x -= dx
            move.y -= dy
            enqueue(.move(dx: dx, dy: dy))
        }
        if scroll.x != 0 || scroll.y != 0 {
            enqueue(.scroll(dx: scroll.x, dy: scroll.y))
            scroll = (0, 0)
        }
    }

    private func armTimeout(_ id: UUID) {
        timeoutTask?.cancel()
        timeoutTask = Task { @MainActor [weak self] in
            do { try await Task.sleep(for: .seconds(2)) } catch { return }
            guard let self, self.generation == id, self.state != .authed else { return }
            self.disconnected()
        }
    }

    private func disconnected() {
        resetConnection()
        state = .disconnected
        retryTask = Task { @MainActor [weak self] in
            do { try await Task.sleep(for: .seconds(1)) } catch { return }
            self?.connect()
        }
    }

    private func resetConnection() {
        generation = UUID()
        discovery.cancelResolution()
        timeoutTask?.cancel()
        retryTask?.cancel()
        receiveTask?.cancel()
        sendTask?.cancel()
        socket?.cancel(with: .goingAway, reason: nil)
        socket = nil
        sendTask = nil
        outgoing.removeAll()
        move = (0, 0)
        scroll = (0, 0)
    }
}
