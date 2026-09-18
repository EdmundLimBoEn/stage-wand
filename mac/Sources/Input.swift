import Foundation
@MainActor enum Input { static func apply(_ c: Command) {} ; static func accessibilityGranted(prompt: Bool) -> Bool { false }; static func selfTest() {} }
