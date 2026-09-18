import SwiftUI

struct SquareUnlockView: View {
    let onUnlock: () -> Void
    @State private var trace = SquareUnlock()
    @State private var tracing = false
    @State private var retry = false

    var body: some View {
        VStack(spacing: 20) {
            Label("Touch controls locked", systemImage: "lock.fill")
                .font(.title2.bold())
            Text("Use the volume buttons to change slides.")
                .foregroundStyle(.secondary)
            GeometryReader { geometry in
                let side = min(geometry.size.width - 48, geometry.size.height - 48, 240)
                let origin = CGPoint(x: (geometry.size.width - side) / 2,
                                     y: (geometry.size.height - side) / 2)
                let square = Path { path in
                    path.move(to: origin)
                    path.addLine(to: CGPoint(x: origin.x + side, y: origin.y))
                    path.addLine(to: CGPoint(x: origin.x + side, y: origin.y + side))
                    path.addLine(to: CGPoint(x: origin.x, y: origin.y + side))
                    path.closeSubpath()
                }
                ZStack {
                    square.stroke(.secondary.opacity(0.4), style: StrokeStyle(lineWidth: 5, dash: [8, 6]))
                    square.trim(from: 0, to: trace.progress / 4)
                        .stroke(trace.failed ? Color.orange : Color.mint, style: StrokeStyle(lineWidth: 7, lineCap: .round))
                    ForEach(0..<4) { corner in
                        Text("\(corner + 1)")
                            .font(.headline.monospacedDigit())
                            .frame(width: 32, height: 32)
                            .background(Color(UIColor.systemBackground), in: Circle())
                            .overlay(Circle().stroke(corner == 0 ? Color.mint : Color.secondary, lineWidth: 2))
                            .position(x: origin.x + (corner == 1 || corner == 2 ? side : 0),
                                      y: origin.y + (corner >= 2 ? side : 0))
                    }
                    Text("1 → 2 → 3 → 4 → 1")
                        .font(.caption.bold())
                        .position(x: geometry.size.width / 2, y: geometry.size.height / 2)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .contentShape(Rectangle())
                .highPriorityGesture(DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        if !tracing {
                            trace.reset()
                            tracing = true
                            retry = false
                            trace.add(x: (value.startLocation.x - origin.x) / side,
                                      y: (value.startLocation.y - origin.y) / side,
                                      at: value.time.timeIntervalSinceReferenceDate)
                        }
                        trace.add(x: (value.location.x - origin.x) / side,
                                  y: (value.location.y - origin.y) / side,
                                  at: value.time.timeIntervalSinceReferenceDate)
                    }
                    .onEnded { value in
                        let unlocked = trace.finish(x: (value.location.x - origin.x) / side,
                                                    y: (value.location.y - origin.y) / side,
                                                    at: value.time.timeIntervalSinceReferenceDate)
                        tracing = false
                        trace.reset()
                        retry = !unlocked
                        if unlocked { onUnlock() }
                    })
            }
            .frame(height: 292)
            .accessibilityLabel("Trace a square clockwise from the top left to unlock")
            Text(retry ? "Try again slowly, keeping your finger on all four edges." : "Start at 1. Slowly trace the square clockwise in one continuous stroke.")
                .font(.callout)
            Text("Complete the square to unlock controls, settings and pairing.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .multilineTextAlignment(.center)
    }
}
