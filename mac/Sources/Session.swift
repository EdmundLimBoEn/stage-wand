import Darwin
import SwiftUI

@MainActor
final class Session: ObservableObject {
    @Published var code = "0000"
    @Published var peer: String?
    @Published var port: UInt16 = 8787
    @Published var axGranted = false
    @Published var lanIP: String?
    @Published var startupError: String?
    var server: Server?
    private var refreshTimer: Timer?

    init() {
        rotateCode()
        lanIP = Self.localAddress()
    }

    func rotateCode() {
        let previous = code
        repeat {
            code = String(format: "%04d", Int.random(in: 0...9999))
        } while code == previous
    }

    func kick() {
        rotateCode()
        server?.kick()
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
            }
        }
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
