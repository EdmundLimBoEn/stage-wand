import AppKit
import CoreImage.CIFilterBuiltins
import SwiftUI

@MainActor
struct StageWandApp: App {
    @NSApplicationDelegateAdaptor(StageWandAppDelegate.self) private var appDelegate
    @StateObject private var session: Session

    init() {
        NSApplication.shared.setActivationPolicy(.accessory)
        print("NSApp.activationPolicy() == .accessory: \(NSApp.activationPolicy() == .accessory)")
        let session = Session()
        let server = Server(session: session) { command in
            Task { @MainActor in
                Input.apply(command)
            }
        }
        session.server = server
        do {
            session.port = try server.start()
        } catch {
            session.startupError = error.localizedDescription
            print("Stage Wand server failed: \(error)")
        }
        _session = StateObject(wrappedValue: session)
        appDelegate.session = session
        Task { @MainActor in
            session.monitorAccessibility()
        }
    }

    var body: some Scene {
        MenuBarExtra("Stage Wand", systemImage: "wand.and.rays") {
            VStack(alignment: .leading, spacing: 14) {
                PairingDetails(session: session)
                SwiftUI.Button("Show Pairing Window") { appDelegate.showPairingWindow() }
                SessionControls(session: session)
            }
            .padding(20)
            .frame(width: 310)
        }
        .menuBarExtraStyle(.window)
    }
}

@MainActor
final class StageWandAppDelegate: NSObject, NSApplicationDelegate {
    var session: Session?
    private var pairingWindow: NSWindow?

    func applicationDidFinishLaunching(_ notification: Notification) {
        showPairingWindow()
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        showPairingWindow()
        return true
    }

    func showPairingWindow() {
        guard let session else { return }
        if pairingWindow == nil {
            let window = NSWindow(
                contentRect: NSRect(x: 0, y: 0, width: 400, height: 640),
                styleMask: [.titled, .closable, .miniaturizable, .resizable],
                backing: .buffered,
                defer: false
            )
            window.title = "Stage Wand Pairing"
            window.isReleasedWhenClosed = false
            window.contentView = NSHostingView(rootView: PairingWindow(session: session))
            window.contentMinSize = NSSize(width: 400, height: 400)
            window.setContentSize(NSSize(width: 400, height: 640))
            window.center()
            pairingWindow = window
        }
        pairingWindow?.deminiaturize(nil)
        pairingWindow?.makeKeyAndOrderFront(nil)
        NSApplication.shared.activate(ignoringOtherApps: true)
    }
}

private struct PairingWindow: View {
    @ObservedObject var session: Session

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                PairingDetails(session: session)
                Text("Nearby Mac · low latency")
                    .font(.headline)
                Text("Keep Wi-Fi and Bluetooth on for both devices. On your phone, draw a square to unlock Stage Wand, choose Use nearby Mac in Settings, then enter the pairing code above. A tunnel is optional.")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                if let tunnelURL = session.tunnelURL {
                    TunnelPairing(tunnelURL: tunnelURL)
                }
                Text(session.tunnelURL != nil
                     ? "For the optional tunnel, an internet connection on both devices is enough. Scan the QR code with iPhone Camera, draw a square to unlock Stage Wand, then enter the displayed pairing code in Settings."
                     : "Open Stage Wand on your phone using the same Wi-Fi and draw a square to unlock it. Your phone discovers this Mac automatically; enter the pairing code in Settings. If discovery fails, enter the Mac address shown above.")
                    .font(.callout)
                    .foregroundStyle(.secondary)
                Text("You can close this window. Stage Wand stays in the menu bar; choose Show Pairing Window to return.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                SessionControls(session: session)
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(minWidth: 400, minHeight: 400)
    }
}

private struct TunnelPairing: View {
    let tunnelURL: String

    private var qrImage: NSImage? {
        var components = URLComponents()
        components.scheme = "stagewand"
        components.host = "connect"
        components.queryItems = [URLQueryItem(name: "url", value: tunnelURL)]
        guard let link = components.url?.absoluteString else { return nil }
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(link.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let scaled = output.transformed(by: CGAffineTransform(scaleX: 8, y: 8))
        guard let image = CIContext().createCGImage(scaled, from: scaled.extent) else { return nil }
        return NSImage(cgImage: image, size: NSSize(width: scaled.extent.width, height: scaled.extent.height))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Divider()
            Text("Connect through tunnel")
                .font(.headline)
            Text("Scan with iPhone Camera, then enter pairing code")
                .font(.callout)
            if let image = qrImage {
                Image(nsImage: image)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .frame(width: 256, height: 256)
                    .padding(16)
                    .background(.white)
                    .accessibilityLabel("Scan to open Stage Wand with this Mac's tunnel address")
            }
            Text(URL(string: tunnelURL)?.host ?? "Tunnel")
                .font(.caption)
                .foregroundStyle(.secondary)
                .textSelection(.enabled)
            SwiftUI.Button("Copy tunnel URL") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(tunnelURL, forType: .string)
            }
        }
    }
}

private struct PairingDetails: View {
    @ObservedObject var session: Session

    var body: some View {
        Text("Stage Wand")
            .font(.headline)
        VStack(alignment: .leading, spacing: 4) {
            Text("PAIRING CODE")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(session.code)
                .font(.system(size: 42, weight: .semibold, design: .monospaced))
                .textSelection(.enabled)
        }
        Text(session.lanIP.map { "\($0):\(session.port)" } ?? "No LAN address · port \(session.port)")
            .font(.system(.callout, design: .monospaced))
            .textSelection(.enabled)
        Label(session.axGranted ? "Accessibility granted" : "Accessibility required",
              systemImage: session.axGranted ? "checkmark.circle.fill" : "exclamationmark.circle.fill")
            .foregroundStyle(session.axGranted ? Color.green : Color.red)
        Text(session.peer.map { "Connected: \($0)" } ?? "Waiting for phone")
            .foregroundStyle(.secondary)
        if let error = session.startupError {
            Text("Server unavailable: \(error)")
                .font(.caption)
                .foregroundStyle(.red)
        }
    }
}

private struct SessionControls: View {
    @ObservedObject var session: Session

    var body: some View {
        Divider()
        HStack {
            SwiftUI.Button("Kick") { session.kick() }
            Spacer()
            SwiftUI.Button("Quit") { NSApplication.shared.terminate(nil) }
        }
    }
}
