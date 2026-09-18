import SwiftUI
@MainActor final class Link: ObservableObject {
 enum State: Equatable { case searching, connecting(String), enterCode, authed, disconnected, localNetworkDenied }
 @Published var state: State = .searching
 @Published var macName: String?
 func send(_ c: Command) {}
 func start() {}
}
