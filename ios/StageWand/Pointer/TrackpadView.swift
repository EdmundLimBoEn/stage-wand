import SwiftUI
struct TrackpadView: UIViewRepresentable {
 let armed: Bool
 let sensitivity: Double
 let onCommand: (Command) -> Void
 func makeUIView(context: Context) -> UIView { UIView() }
 func updateUIView(_ uiView: UIView, context: Context) {}
}
