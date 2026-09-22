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
    @AppStorage("transport") private var transport: String = "bluetooth"
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
                    Picker("Connection", selection: $transport) {
                        Text("Bluetooth direct").tag("bluetooth")
                        Text("Wi‑Fi nearby").tag("wifi")
                    }
                    .pickerStyle(.segmented)
                    SwiftUI.Button("Use direct connection (clear tunnel URL)") { manualHost = "" }
                    Text("Bluetooth direct needs no Wi‑Fi and is the smoothest for the pointer. Wi‑Fi nearby uses your network or Apple peer-to-peer Wi‑Fi. Allow Bluetooth on both devices when asked.")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    TextField("Pairing code", text: $pairingCode)
                        .keyboardType(.numberPad)
                        .textContentType(.oneTimeCode)
                    TextField("Computer address or tunnel URL", text: $manualHost)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    if !manualHost.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
                       ConnectionURL.parse(manualHost) == nil {
                        Text("Enter a valid computer address or ws, wss, http or https URL without a username, password or fragment.")
                            .font(.caption)
                            .foregroundStyle(.orange)
                    }
                }
                Section {
                    Text("Find the pairing code in Stage Wand on your computer. Paste a tunnel URL when the network blocks local connections, or enter a local host:port. Leave the address empty for automatic discovery. After five failed pairing attempts, wait 30 seconds or refresh the code on your computer.")
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
