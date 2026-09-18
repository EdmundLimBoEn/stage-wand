import UIKit

@MainActor
enum Haptics {
    private static let generator = UIImpactFeedbackGenerator(style: .medium)

    static func tick() {
        generator.impactOccurred()
        generator.prepare()
    }
}
