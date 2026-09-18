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

    static func tick() {
        generator.impactOccurred()
        generator.prepare()
    }
}
