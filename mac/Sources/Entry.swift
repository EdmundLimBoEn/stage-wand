import Foundation
import SwiftUI
@main enum Entry {
    @MainActor static func main() {
        if CommandLine.arguments.contains("--selftest") { Input.selfTest(); return }
        if CommandLine.arguments.contains("--serve") {
            let session = Session(); session.code = "0000"
            let server = Server(session: session) { command in
                print("COMMAND " + String(data: (try? JSONEncoder().encode(command)) ?? Data(), encoding: .utf8)!)
                fflush(stdout)
            }
            do { let port = try server.start(); print("PORT \(port)"); fflush(stdout) }
            catch { print("Server failed: \(error)"); exit(1) }
            withExtendedLifetime(server) { RunLoop.main.run() }; return
        }
        StageWandApp.main()
    }
}
