import AppKit
import SwiftUI

@MainActor
struct StageWandApp: App {
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
        Task { @MainActor in
            session.monitorAccessibility()
        }
    }

    var body: some Scene {
        MenuBarExtra("Stage Wand", systemImage: "wand.and.rays") {
            VStack(alignment: .leading, spacing: 14) {
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
                Text(session.peer ?? "waiting")
                    .foregroundStyle(.secondary)
                if let error = session.startupError {
                    Text("Server unavailable: \(error)")
                        .font(.caption)
                        .foregroundStyle(.red)
                }
                Divider()
                HStack {
                    SwiftUI.Button("Kick") { session.kick() }
                    Spacer()
                    SwiftUI.Button("Quit") { NSApplication.shared.terminate(nil) }
                }
            }
            .padding(20)
            .frame(width: 310)
        }
        .menuBarExtraStyle(.window)
    }
}
