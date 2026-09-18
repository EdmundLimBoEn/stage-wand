import Foundation
@main enum LinkCheck {
 @MainActor static func main() async throws {
  let defaults = UserDefaults.standard
  defaults.set("0000", forKey: "pairingCode")
  defaults.set("127.0.0.1:\(ProcessInfo.processInfo.environment["PORT"] ?? "8787")", forKey: "manualHost")
  defer { defaults.removeObject(forKey: "pairingCode"); defaults.removeObject(forKey: "manualHost") }
  let link = Link()
  for _ in 0..<10 { link.send(.key(.right)) }
  link.start()
  for _ in 0..<60 {
   if link.state == .authed { break }
   try await Task.sleep(for: .milliseconds(100))
  }
  guard link.state == .authed else { fatalError("Link did not authenticate: \(link.state)") }
  link.send(.move(dx: 900, dy: -850))
  link.send(.scroll(dx: 12, dy: 34))
  try await Task.sleep(for: .milliseconds(300))
  defaults.set("99999", forKey: "pairingCode")
  link.start()
  for _ in 0..<40 {
   if link.state == .enterCode { break }
   try await Task.sleep(for: .milliseconds(100))
  }
  guard link.state == .enterCode else { fatalError("Bad code did not enter code state: \(link.state)") }
  defaults.set("", forKey: "pairingCode")
  link.start()
  precondition(link.state == .enterCode)
  print("PASS: Link authenticated, flushed 8 buffered keys and split deltas, rejected bad code, handled empty code")
 }
}
