import UIKit

@MainActor
enum Haptics {
    static func tick() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
    }
}
