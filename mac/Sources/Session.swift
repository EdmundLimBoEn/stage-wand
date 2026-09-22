import Darwin
import SwiftUI

@MainActor
final class Session: ObservableObject {
    @Published var code = "0000"
    @Published private(set) var peer: String?
    @Published var port: UInt16 = 8787
    @Published var axGranted = false
    @Published var lanIP: String?
    @Published var startupError: String?
    @Published var bluetoothError: String?
    @Published var tunnelURL: String?
    var server: Server?
    var bluetooth: BluetoothServer?
    private var refreshTimer: Timer?
    private var authentication = AuthenticationLimiter()
    private let ownership = PeerOwnership()

    init() {
        rotateCode()
        lanIP = Self.localAddress()
        refreshTunnelURL()
    }

    func rotateCode() {
        let previous = code
        repeat {
            code = String(format: "%04d", Int.random(in: 0...9999))
        } while code == previous
        authentication.reset()
    }

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

    func kick() {
        rotateCode()
        server?.kick()
        bluetooth?.kick()
        peer = nil
    }

    func monitorAccessibility() {
        guard refreshTimer == nil else { return }
        axGranted = Input.accessibilityGranted(prompt: true)
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.axGranted = Input.accessibilityGranted(prompt: false)
                self.lanIP = Self.localAddress()
                self.refreshTunnelURL()
            }
        }
    }

    private func refreshTunnelURL() {
        let file = FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent(".config/stage-wand/tunnel-url")
        let value = (try? String(contentsOf: file, encoding: .utf8))?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let validURL = value.flatMap { value -> String? in
            guard let url = URL(string: value), url.scheme == "wss",
                  let host = url.host, !host.isEmpty,
                  url.user == nil, url.password == nil else { return nil }
            return value
        }
        if tunnelURL != validURL { tunnelURL = validURL }
    }

    private static func localAddress() -> String? {
        var interfaces: UnsafeMutablePointer<ifaddrs>?
        guard getifaddrs(&interfaces) == 0, let first = interfaces else { return nil }
        defer { freeifaddrs(first) }
        var addresses: [(name: String, address: String)] = []
        var current: UnsafeMutablePointer<ifaddrs>? = first
        while let interface = current {
            defer { current = interface.pointee.ifa_next }
            let entry = interface.pointee
            guard let address = entry.ifa_addr,
                  address.pointee.sa_family == UInt8(AF_INET),
                  entry.ifa_flags & UInt32(IFF_UP) != 0,
                  entry.ifa_flags & UInt32(IFF_LOOPBACK) == 0 else { continue }
            var host = [CChar](repeating: 0, count: Int(NI_MAXHOST))
            guard getnameinfo(address, socklen_t(address.pointee.sa_len), &host,
                              socklen_t(host.count), nil, 0, NI_NUMERICHOST) == 0 else { continue }
            let ip = String(decoding: host.prefix { $0 != 0 }.map { UInt8(bitPattern: $0) }, as: UTF8.self)
            addresses.append((String(cString: entry.ifa_name), ip))
        }
        return addresses.first { $0.name == "en0" }?.address
            ?? addresses.first { $0.name.hasPrefix("en") }?.address
            ?? addresses.first?.address
    }
}
