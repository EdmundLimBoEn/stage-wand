import AudioToolbox
import UIKit

@MainActor
enum Haptics {
    private static let generator = UIImpactFeedbackGenerator(style: .medium)
    private static let motionGenerator = UIImpactFeedbackGenerator(style: .light)
    private static var lastMotionTime: CFTimeInterval = 0

    static func motion() {
        let now = CACurrentMediaTime()
        guard now - lastMotionTime >= 0.08 else { return }
        lastMotionTime = now
        motionGenerator.impactOccurred(intensity: 0.45)
        motionGenerator.prepare()
    }

    static let enabledKey = "hapticFeedback"

    static var enabled: Bool {
        UserDefaults.standard.object(forKey: enabledKey) as? Bool ?? true
    }

    /// Volume-button click confirmation. UIKit feedback generators are silent while the app
    /// is in the background or the screen is locked, so fall back to the short system "peek"
    /// vibration there; it plays while the volume-listener audio session keeps the app alive.
    static func click() {
        guard enabled else { return }
        if UIApplication.shared.applicationState == .active {
            clickGenerator.impactOccurred()
            clickGenerator.prepare()
        } else {
            AudioServicesPlaySystemSound(1519)
        }
    }

    private static let clickGenerator = UIImpactFeedbackGenerator(style: .light)

    static func tick() {
        generator.impactOccurred()
        generator.prepare()
    }
}
