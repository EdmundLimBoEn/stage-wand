import SwiftUI

enum PointerMode: String, CaseIterable {
    case trackpad
}

@MainActor
struct SettingsView: View {
    @AppStorage("sensitivity") private var sensitivity: Double = 1.0
    @AppStorage("pairingCode") private var pairingCode: String = ""
    @AppStorage("manualHost") private var manualHost: String = ""
    @AppStorage("pointerMode") private var pointerMode: String = "trackpad"
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Form {
                Section("Pointer") {
                    Picker("Mode", selection: $pointerMode) {
                        ForEach(PointerMode.allCases, id: \.rawValue) { mode in
                            Text(mode.rawValue.capitalized).tag(mode.rawValue)
                        }
                    }
                    HStack {
                        Text("Sensitivity")
                        Spacer()
                        Text(sensitivity, format: .number.precision(.fractionLength(1)))
                            .monospacedDigit()
                    }
                    Slider(value: $sensitivity, in: 0.5...3, step: 0.1)
                        .accessibilityLabel("Pointer sensitivity")
                }
                Section("Pairing") {
                    SwiftUI.Button("Use nearby Mac (low latency)") { manualHost = "" }
                    Text("Keep Wi-Fi and Bluetooth on on both devices. Nearby mode can connect directly without a shared router when peer-to-peer Wi-Fi is available.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    TextField("Pairing code", text: $pairingCode)
                        .keyboardType(.numberPad)
                        .textContentType(.oneTimeCode)
                    TextField("Mac address or tunnel URL", text: $manualHost)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    if !manualHost.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                       ConnectionURL.parse(manualHost) == nil {
                        Text("Enter a valid Mac address or ws, wss, http or https URL without a username, password or fragment.")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }
                }
                Section {
                    Text("Find the pairing code in the Stage Wand menu on your Mac. Paste a tunnel URL when the network blocks local connections, or enter a local host:port. Leave the address empty for automatic discovery.")
                }
            }
            .navigationTitle("Settings")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    SwiftUI.Button("Done") { dismiss() }
                }
            }
        }
    }
}
