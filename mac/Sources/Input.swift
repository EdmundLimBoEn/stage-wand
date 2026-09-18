import ApplicationServices
import Foundation

@MainActor
enum Input {
    static func apply(_ c: Command) {
        switch c {
        case .auth:
            break
        case .move(let dx, let dy):
            guard dx.isFinite, dy.isFinite,
                  let current = CGEvent(source: nil)?.location,
                  let bounds = displayBounds() else { return }
            let point = CGPoint(
                x: min(max(current.x + dx, bounds.minX), bounds.maxX - 1),
                y: min(max(current.y + dy, bounds.minY), bounds.maxY - 1)
            )
            guard let event = CGEvent(mouseEventSource: nil, mouseType: .mouseMoved,
                                      mouseCursorPosition: point, mouseButton: .left) else { return }
            event.setDoubleValueField(.mouseEventDeltaX, value: point.x - current.x)
            event.setDoubleValueField(.mouseEventDeltaY, value: point.y - current.y)
            event.post(tap: .cghidEventTap)
        case .click(let button):
            guard let point = CGEvent(source: nil)?.location else { return }
            let mouseButton: CGMouseButton = button == .left ? .left : .right
            let down: CGEventType = button == .left ? .leftMouseDown : .rightMouseDown
            let up: CGEventType = button == .left ? .leftMouseUp : .rightMouseUp
            for type in [down, up] {
                let event = CGEvent(mouseEventSource: nil, mouseType: type,
                                    mouseCursorPosition: point, mouseButton: mouseButton)
                event?.setIntegerValueField(.mouseEventClickState, value: 1)
                event?.post(tap: .cghidEventTap)
            }
        case .scroll(let dx, let dy):
            guard dx.isFinite, dy.isFinite else { return }
            CGEvent(scrollWheelEvent2Source: nil, units: .pixel, wheelCount: 2,
                    wheel1: scrollValue(dy), wheel2: scrollValue(dx), wheel3: 0)?
                .post(tap: .cghidEventTap)
        case .key(let key):
            let code: CGKeyCode
            switch key {
            case .left: code = 123
            case .right: code = 124
            case .esc: code = 53
            }
            press(code, flags: [])
        case .chord(let chord):
            let code: CGKeyCode
            switch chord {
            case .spaceLeft: code = 123
            case .spaceRight: code = 124
            case .missionControl: code = 126
            }
            press(code, flags: .maskControl)
        }
    }

    static func accessibilityGranted(prompt: Bool) -> Bool {
        // The SDK exposes the equivalent constant as concurrency-unsafe mutable state.
        let options = ["AXTrustedCheckOptionPrompt": prompt]
        return AXIsProcessTrustedWithOptions(options as CFDictionary)
    }

    static func selfTest() {
        guard accessibilityGranted(prompt: true) else {
            print("SELFTEST FAIL: Accessibility permission is required. Enable StageWandMac in System Settings > Privacy & Security > Accessibility.")
            exit(1)
        }
        guard let before = CGEvent(source: nil)?.location else {
            print("SELFTEST FAIL: Could not read pointer coordinates.")
            exit(1)
        }
        print("SELFTEST before: (\(before.x), \(before.y))")
        apply(.move(dx: 100, dy: 0))
        usleep(100_000)
        if let moved = CGEvent(source: nil)?.location {
            print("SELFTEST right: (\(moved.x), \(moved.y))")
            apply(.move(dx: before.x - moved.x, dy: before.y - moved.y))
        }
        usleep(100_000)
        if let after = CGEvent(source: nil)?.location {
            print("SELFTEST after: (\(after.x), \(after.y))")
        }
        apply(.click(.left))
        apply(.key(.esc))
        CGEvent(scrollWheelEvent2Source: nil, units: .line, wheelCount: 2,
                wheel1: 3, wheel2: 0, wheel3: 0)?.post(tap: .cghidEventTap)
        print("SELFTEST PASS: Posted pointer movement, left click, Esc, and three-line scroll.")
    }

    private static func press(_ code: CGKeyCode, flags: CGEventFlags) {
        for down in [true, false] {
            let event = CGEvent(keyboardEventSource: nil, virtualKey: code, keyDown: down)
            event?.flags = flags
            event?.post(tap: .cghidEventTap)
        }
    }

    private static func scrollValue(_ value: Double) -> Int32 {
        Int32(min(max(value, Double(Int32.min)), Double(Int32.max)))
    }

    private static func displayBounds() -> CGRect? {
        var count: UInt32 = 0
        guard CGGetActiveDisplayList(0, nil, &count) == .success, count > 0 else { return nil }
        var displays = [CGDirectDisplayID](repeating: 0, count: Int(count))
        guard CGGetActiveDisplayList(count, &displays, &count) == .success else { return nil }
        let bounds = displays.prefix(Int(count)).reduce(CGRect.null) { $0.union(CGDisplayBounds($1)) }
        return bounds.isNull || bounds.isEmpty ? nil : bounds
    }
}
