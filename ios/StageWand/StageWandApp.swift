import SwiftUI

@main
@MainActor
struct StageWandApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                .preferredColorScheme(.dark)
                .onOpenURL { url in
                    guard let destination = ConnectionURL.pairingDestination(from: url) else { return }
                    let defaults = UserDefaults.standard
                    if ConnectionURL.parse(defaults.string(forKey: "manualHost") ?? "") != destination {
                        // Clear credentials before publishing a destination that can trigger reconnect.
                        defaults.set("", forKey: "pairingCode")
                    }
                    defaults.set(destination.absoluteString, forKey: "manualHost")
                }
        }
    }
}
