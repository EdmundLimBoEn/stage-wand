import Foundation
@main enum ProtocolCheck {
 static func main() throws {
  let encoder = JSONEncoder(), decoder = JSONDecoder()
  let commands: [Command] = [.auth(code: "4821"), .move(dx: 3.2, dy: -1), .click(.left), .click(.right), .scroll(dx: 0, dy: 40), .key(.esc), .key(.left), .key(.right), .chord(.spaceRight), .chord(.spaceLeft), .chord(.missionControl)]
  for command in commands {
   let data = try encoder.encode(command)
   let decoded = try decoder.decode(Command.self, from: data)
   precondition(decoded == command)
   let object = try JSONSerialization.jsonObject(with: data) as! [String: Any]
   precondition(object["t"] is String)
  }
  for reply in [Reply.status, .bye(reason: "displaced")] {
   let decoded = try decoder.decode(Reply.self, from: encoder.encode(reply))
   precondition(decoded == reply)
  }
  for bad in ["{\"t\":\"move\",\"dx\":1e999,\"dy\":0}", "{\"t\":\"unknown\"}"] {
   do { _ = try decoder.decode(Command.self, from: Data(bad.utf8)); fatalError("Invalid frame accepted") } catch {}
  }
  do { _ = try encoder.encode(Command.move(dx: .nan, dy: 0)); fatalError("NaN encoded") } catch {}
  print("PASS: command/reply round trips, flat discriminator, invalid frames")
 }
}
