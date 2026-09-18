import SwiftUI

@MainActor
struct ContentView: View {
    @StateObject private var link = Link()
    @State private var volume: Volume?
    @State private var armed = false
    @State private var showingSettings = false
    @AppStorage("sensitivity") private var sensitivity: Double = 1.0
    @AppStorage("pairingCode") private var pairingCode: String = ""
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.openURL) private var openURL

    var body: some View {
        NavigationStack {
            VStack(spacing: 16) {
                connectionPill
                if link.state == .enterCode {
                    HStack {
                        TextField("Enter Mac code", text: $pairingCode)
                            .keyboardType(.numberPad)
                            .textContentType(.oneTimeCode)
                            .textFieldStyle(.roundedBorder)
                            .accessibilityLabel("Pairing code")
                        SwiftUI.Button("Pair") {
                            link.start()
                            Haptics.tick()
                        }
                            .disabled(pairingCode.isEmpty)
                    }
                }
                Toggle(isOn: $armed) {
                    Label(armed ? "ARMED" : "ARM POINTER", systemImage: armed ? "hand.draw.fill" : "hand.draw")
                        .font(.headline)
                }
                .tint(.mint)

                TrackpadView(armed: armed, sensitivity: sensitivity) { command in
                    guard armed else { return }
                    send(command)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .frame(minHeight: 140)
                .background(.white.opacity(0.06), in: RoundedRectangle(cornerRadius: 24))
                .overlay(alignment: .top) {
                    Text(armed ? "Drag to point · Tap to click" : "Arm to use the trackpad")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .padding()
                        .allowsHitTesting(false)
                }
                .clipShape(RoundedRectangle(cornerRadius: 24))
                .accessibilityLabel("Trackpad")

                HStack(spacing: 12) {
                    commandButton("←", label: "Left arrow", command: .key(.left))
                    commandButton("→", label: "Right arrow", command: .key(.right))
                    commandButton("ESC", label: "Escape", command: .key(.esc))
                }
                HStack(spacing: 12) {
                    commandButton("L CLICK", label: "Left click", command: .click(.left))
                    commandButton("R CLICK", label: "Right click", command: .click(.right))
                }
                .disabled(!armed)
                HStack(spacing: 12) {
                    commandButton("PREV", label: "Previous slide", command: .key(.left), large: true)
                    commandButton("NEXT", label: "Next slide", command: .key(.right), large: true)
                }
                TimelineView(.periodic(from: .now, by: 0.5)) { _ in
                    Text("Volume presses: \(volume?.pressCount ?? 0)")
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            .padding()
            .navigationTitle("Stage Wand")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    SwiftUI.Button { showingSettings = true } label: {
                        Image(systemName: "gearshape")
                    }
                    .accessibilityLabel("Settings")
                }
            }
            .sheet(isPresented: $showingSettings) { SettingsView() }
            .task {
                guard volume == nil else { return }
                volume = Volume(
                    onUp: { send(.key(.right)) },
                    onDown: { send(.key(.left)) }
                )
                volume?.start()
                link.start()
            }
            .onChange(of: scenePhase) { _, phase in
                if phase == .background { armed = false }
                if phase == .active { volume?.start() }
            }
        }
    }

    private var connectionPill: some View {
        SwiftUI.Button {
            switch link.state {
            case .enterCode: showingSettings = true
            case .localNetworkDenied:
                if let url = URL(string: UIApplication.openSettingsURLString) { openURL(url) }
            case .disconnected: link.start()
            default: break
            }
        } label: {
            HStack(spacing: 8) {
                Circle().fill(link.state == .authed ? Color.mint : Color.orange)
                    .frame(width: 8, height: 8)
                Text(connectionText).font(.subheadline)
                Spacer(minLength: 0)
            }
            .padding(12)
            .background(.white.opacity(0.08), in: Capsule())
        }
        .buttonStyle(.plain)
    }

    private var connectionText: String {
        switch link.state {
        case .searching: "Searching for your Mac…"
        case .connecting(let host): "Connecting to \(link.macName ?? host)…"
        case .enterCode: "Enter code · \(link.macName ?? "Mac")"
        case .authed: "Connected · \(link.macName ?? "Mac")"
        case .disconnected: "Disconnected · Tap to reconnect"
        case .localNetworkDenied: "Local network denied, fix in Settings"
        }
    }

    private func commandButton(_ title: String, label: String, command: Command, large: Bool = false) -> some View {
        SwiftUI.Button { send(command) } label: {
            Text(title)
                .font(large ? .title2.bold() : .headline)
                .frame(maxWidth: .infinity, minHeight: large ? 68 : 44)
                .background(large ? Color.mint.opacity(0.2) : Color.white.opacity(0.08), in: RoundedRectangle(cornerRadius: 14))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(label)
    }

    private func send(_ command: Command) {
        switch command {
        case .move, .click, .scroll, .chord:
            guard armed else { return }
        case .key, .auth: break
        }
        link.send(command)
        Haptics.tick()
    }
}
