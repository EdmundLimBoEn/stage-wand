import SwiftUI
@MainActor final class Session: ObservableObject {
 @Published var code = "0000"
 @Published var peer: String?
 @Published var port: UInt16 = 8787
 @Published var axGranted = false
 func rotateCode() {}
 func kick() {}
}
