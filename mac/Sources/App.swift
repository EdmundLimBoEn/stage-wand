import AppKit
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
                contentRect: NSRect(x: 0, y: 0, width: 400, height: 400),
                styleMask: [.titled, .closable, .miniaturizable],
                backing: .buffered,
                defer: false
            )
            window.title = "Stage Wand Pairing"
            window.isReleasedWhenClosed = false
            window.contentView = NSHostingView(rootView: PairingWindow(session: session))
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
        VStack(alignment: .leading, spacing: 18) {
            PairingDetails(session: session)
            Text("Open Stage Wand on your phone using the same Wi-Fi and draw a square to unlock it. Your phone discovers this Mac automatically; enter the pairing code in Settings. If discovery fails, enter the Mac address shown above.")
                .font(.callout)
                .foregroundStyle(.secondary)
            Text("You can close this window. Stage Wand stays in the menu bar; choose Show Pairing Window to return.")
                .font(.caption)
                .foregroundStyle(.secondary)
            SessionControls(session: session)
        }
        .padding(24)
        .frame(width: 400)
        .fixedSize(horizontal: false, vertical: true)
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
