import Foundation
@MainActor final class Server {
 init(session: Session, onCommand: @escaping @Sendable (Command) -> Void) {}
 func start() throws -> UInt16 { 8787 }
 func kick() {}
}
