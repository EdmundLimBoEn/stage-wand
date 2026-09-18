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
                    TextField("Pairing code", text: $pairingCode)
                        .keyboardType(.numberPad)
                        .textContentType(.oneTimeCode)
                    TextField("Manual host or host:port", text: $manualHost)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }
                Section {
                    Text("Find the pairing code and port in the Stage Wand menu on your Mac. Leave manual host empty to discover your Mac automatically.")
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
