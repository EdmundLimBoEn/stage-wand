import SwiftUI
import UIKit

@MainActor
struct TrackpadView: UIViewRepresentable {
    let armed: Bool
    let sensitivity: Double
    let onCommand: (Command) -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }

    func makeUIView(context: Context) -> UIView {
        let view = UIView()
        view.backgroundColor = .clear
        view.isMultipleTouchEnabled = true
        view.accessibilityLabel = "Trackpad"
        view.accessibilityHint = "Drag to move, tap to click, use two fingers to scroll or right click, and swipe with three fingers to change spaces or open Mission Control."
        context.coordinator.install(on: view)
        return view
    }

    func updateUIView(_ uiView: UIView, context: Context) {
        context.coordinator.update(self)
    }

    static func dismantleUIView(_ uiView: UIView, coordinator: Coordinator) {
        coordinator.stop()
    }

    @MainActor
    final class Coordinator: NSObject {
        private var configuration: TrackpadView
        private var movement = CGPoint.zero
        private var scrolling = CGPoint.zero
        private var displayLink: CADisplayLink?
        private var isPanning = false

        init(_ configuration: TrackpadView) {
            self.configuration = configuration
        }

        func install(on view: UIView) {
            let move = UIPanGestureRecognizer(target: self, action: #selector(pan(_:)))
            move.minimumNumberOfTouches = 1
            move.maximumNumberOfTouches = 1
            let scroll = UIPanGestureRecognizer(target: self, action: #selector(pan(_:)))
            scroll.minimumNumberOfTouches = 2
            scroll.maximumNumberOfTouches = 2
            let left = UITapGestureRecognizer(target: self, action: #selector(tap(_:)))
            left.numberOfTouchesRequired = 1
            let right = UITapGestureRecognizer(target: self, action: #selector(tap(_:)))
            right.numberOfTouchesRequired = 2
            left.require(toFail: right)
            left.require(toFail: move)
            right.require(toFail: scroll)
            for recognizer in [move, scroll, left, right] {
                view.addGestureRecognizer(recognizer)
            }
            for direction: UISwipeGestureRecognizer.Direction in [.left, .right, .up] {
                let swipe = UISwipeGestureRecognizer(target: self, action: #selector(swipe(_:)))
                swipe.numberOfTouchesRequired = 3
                swipe.direction = direction
                view.addGestureRecognizer(swipe)
            }
            let target = DisplayTarget()
            target.coordinator = self
            let link = CADisplayLink(target: target, selector: #selector(DisplayTarget.tick))
            link.preferredFrameRateRange = CAFrameRateRange(minimum: 60, maximum: 60, preferred: 60)
            link.isPaused = true
            link.add(to: .main, forMode: .common)
            displayLink = link
        }

        func update(_ configuration: TrackpadView) {
            self.configuration = configuration
            if !configuration.armed { clear() }
        }

        func stop() {
            displayLink?.invalidate()
            displayLink = nil
            clear()
        }

        private func clear() {
            isPanning = false
            movement = .zero
            scrolling = .zero
            displayLink?.isPaused = true
        }

        @objc private func pan(_ recognizer: UIPanGestureRecognizer) {
            if recognizer.state == .cancelled || recognizer.state == .failed {
                isPanning = false
                flush()
                return
            }
            let delta = recognizer.translation(in: recognizer.view)
            recognizer.setTranslation(.zero, in: recognizer.view)
            guard configuration.armed,
                  [.began, .changed, .ended].contains(recognizer.state) else { return }
            isPanning = recognizer.state != .ended
            let scale = 2.0 * configuration.sensitivity
            let dx = delta.x * scale
            let dy = delta.y * scale
            guard dx.isFinite, dy.isFinite else { return }
            if recognizer.minimumNumberOfTouches == 1 {
                let next = CGPoint(x: movement.x + dx, y: movement.y + dy)
                guard next.x.isFinite, next.y.isFinite else { return }
                movement = next
            } else {
                let next = CGPoint(x: scrolling.x + dx, y: scrolling.y + dy)
                guard next.x.isFinite, next.y.isFinite else { return }
                scrolling = next
            }
            displayLink?.isPaused = false
            if recognizer.state == .ended { flush() }
        }

        @objc private func tap(_ recognizer: UITapGestureRecognizer) {
            guard configuration.armed, recognizer.state == .ended else { return }
            flush()
            configuration.onCommand(.click(recognizer.numberOfTouchesRequired == 1 ? .left : .right))
        }

        @objc private func swipe(_ recognizer: UISwipeGestureRecognizer) {
            guard configuration.armed, recognizer.state == .ended else { return }
            let chord: Chord
            switch recognizer.direction {
            case .left: chord = .spaceLeft
            case .right: chord = .spaceRight
            case .up: chord = .missionControl
            default: return
            }
            flush()
            configuration.onCommand(.chord(chord))
        }

        fileprivate func flush() {
            guard configuration.armed else { clear(); return }
            let move = movement
            let scroll = scrolling
            movement = .zero
            scrolling = .zero
            if !isPanning { displayLink?.isPaused = true }
            // Split large deltas to honor the wire limit without discarding distance.
            var remaining = move
            while remaining != .zero {
                let dx = min(400, max(-400, remaining.x))
                let dy = min(400, max(-400, remaining.y))
                configuration.onCommand(.move(dx: Double(dx), dy: Double(dy)))
                remaining.x -= dx
                remaining.y -= dy
            }
            if scroll != .zero {
                configuration.onCommand(.scroll(dx: Double(scroll.x), dy: Double(scroll.y)))
            }
        }
    }

    @MainActor
    private final class DisplayTarget: NSObject {
        weak var coordinator: Coordinator?
        @objc func tick() { coordinator?.flush() }
    }
}

@MainActor
private struct TrackpadPreview: View {
    @State private var armed = true
    @State private var lines: [String] = []

    var body: some View {
        VStack {
            Toggle("ARM", isOn: $armed)
            Text("Drag / tap · Two fingers: scroll / right click · Three fingers: swipe left, right, up")
                .font(.caption)
            TrackpadView(armed: armed, sensitivity: 1) { command in
                guard let data = try? JSONEncoder().encode(command),
                      let line = String(data: data, encoding: .utf8) else { return }
                print(line)
                lines.append(line)
                if lines.count > 100 { lines.removeFirst(lines.count - 100) }
            }
            .frame(minHeight: 250)
            .background(.quaternary, in: RoundedRectangle(cornerRadius: 20))
            ScrollView {
                Text(lines.joined(separator: "\n"))
                    .font(.system(.caption2, design: .monospaced))
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(height: 200)
        }
        .padding()
    }
}

#Preview { TrackpadPreview() }
