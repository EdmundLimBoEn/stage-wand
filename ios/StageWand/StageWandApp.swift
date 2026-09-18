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
                    UserDefaults.standard.set(destination.absoluteString, forKey: "manualHost")
                }
        }
    }
}
