import Foundation
import Combine
import Network

@MainActor
final class Link: ObservableObject {
    enum State: Equatable {
        case searching, connecting(String), enterCode, authed, disconnected, localNetworkDenied
    }
    @Published var state: State = .searching
    @Published var macName: String?
    @Published private(set) var route = "Nearby"
    private let discovery = Discovery()
    @MainActor
    private enum Transport {
        case remote(URLSessionWebSocketTask)
        case nearby(LocalSocket)
        case bluetooth(BluetoothClient)

        func send(_ data: Data) async throws {
            switch self {
            case .remote(let socket): try await socket.send(.string(String(decoding: data, as: UTF8.self)))
            case .nearby(let socket): try await socket.send(data)
            case .bluetooth(let client): try await client.send(data)
            }
        }

        func receive() async throws -> Data {
            switch self {
            case .remote(let socket):
                switch try await socket.receive() {
                case .data(let data): return data
                case .string(let string): return Data(string.utf8)
                @unknown default: throw CancellationError()
                }
            case .nearby(let socket): return try await socket.receive()
            case .bluetooth(let client): return try await client.receive()
            }
        }

        func cancel() {
            switch self {
            case .remote(let socket): socket.cancel(with: .goingAway, reason: nil)
            case .nearby(let socket): socket.cancel()
            case .bluetooth(let client): client.cancel()
            }
        }
    }
    private var socket: Transport?
    private var receiveTask: Task<Void, Never>?
    private var sendTask: Task<Void, Never>?
    private var retryTask: Task<Void, Never>?
    private var timeoutTask: Task<Void, Never>?
    private var defaultsObserver: AnyCancellable?
    private var generation = UUID()
    private let bluetooth = BluetoothClient()
    private var code = ""
    private var host = ""
    private var transport = "bluetooth"
    private var buffered: [Command] = []
    private var outgoing = CommandQueue()

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
        refreshSettings(force: true)
    }

    func send(_ command: Command) {
        switch command {
        case .auth: return
        case .move(let dx, let dy), .scroll(let dx, let dy):
            guard state == .authed, dx.isFinite, dy.isFinite else { return }
            if dx != 0 || dy != 0 { enqueue(command) }
        case .key, .click, .chord:
            if state == .authed {
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
        let newTransport = UserDefaults.standard.string(forKey: "transport") ?? "bluetooth"
        guard force || newCode != code || newHost != host || newTransport != transport else { return }
        code = newCode
        host = newHost
        transport = newTransport
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
            guard let url = ConnectionURL.parse(host) else { macName = nil; disconnected(); return }
            macName = url.host
            open(url)
            return
        }
        if transport == "bluetooth" {
            route = "Bluetooth"
            macName = nil
            bluetooth.onName = { [weak self] in self?.macName = $0 }
            state = .connecting("Mac")
            begin(.bluetooth(bluetooth))
            return
        }
        discovery.start()
        guard let result = discovery.results.first else { state = .searching; return }
        macName = result.name
        route = "Nearby"
        state = .connecting(result.name)
        let id = generation
        armTimeout(id)
        discovery.resolve(result.endpoint) { [weak self] url, interface in
            guard let self, self.generation == id else { return }
            guard let url else { self.disconnected(); return }
            self.begin(.nearby(LocalSocket(url: url, interface: interface)))
        }
    }

    private func open(_ url: URL) {
        state = .connecting(macName ?? url.host ?? "Mac")
        if url.scheme == "ws" {
            route = "Direct"
            begin(.nearby(LocalSocket(url: url)))
            return
        }
        route = "Secure link"
        let socket = URLSession.shared.webSocketTask(with: url)
        socket.resume()
        begin(.remote(socket))
    }

    private func begin(_ socket: Transport) {
        self.socket = socket
        let id = generation
        armTimeout(id)
        enqueue(.auth(code: code))
        receiveTask = Task { @MainActor [weak self] in
            do {
                while !Task.isCancelled {
                    let data = try await socket.receive()
                    guard let self, self.generation == id else { return }
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
                    guard let command = self.outgoing.popFirst() else { break }
                    let data = try JSONEncoder().encode(command)
                    try await socket.send(data)
                }
                guard let self, self.generation == id else { return }
                self.sendTask = nil
            } catch {
                guard let self, self.generation == id else { return }
                self.disconnected()
            }
        }
    }

    private func armTimeout(_ id: UUID) {
        timeoutTask?.cancel()
        timeoutTask = Task { @MainActor [weak self] in
            do { try await Task.sleep(for: .seconds(10)) } catch { return }
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
        socket?.cancel()
        socket = nil
        sendTask = nil
        outgoing.removeAll()
    }
}
