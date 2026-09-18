import Foundation
@MainActor final class Volume {
 init(onUp: @escaping () -> Void, onDown: @escaping () -> Void) {}
 func start() {}
 func stop() {}
 var pressCount = 0
}
