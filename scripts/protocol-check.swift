import Foundation

@main enum ProtocolCheck {
    struct Fixtures: Decodable {
        let validCommandTexts: [String]
        let commands: [Command]
        let replies: [Reply]
        let invalidCommands: [String]
        let invalidReplies: [String]
        let bluetooth: Bluetooth
        struct Bluetooth: Decodable {
            let service: String
            let command: String
            let reply: String
            let maxFrameBytes: Int
            let minimumCommandBytes: Int
        }
    }

    static func main() throws {
        let encoder = JSONEncoder(), decoder = JSONDecoder()
        let path = CommandLine.arguments.dropFirst().first ?? "Shared/protocol-fixtures.json"
        let data = try Data(contentsOf: URL(fileURLWithPath: path))
        let fixtures = try decoder.decode(Fixtures.self, from: data)
        let raw = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        let rawCommands = raw["commands"] as! [NSDictionary]
        let rawReplies = raw["replies"] as! [NSDictionary]
        for (index, command) in fixtures.commands.enumerated() {
            let encoded = try encoder.encode(command)
            let decoded = try WireProtocol.decodeCommand(encoded)
            precondition(decoded == command)
            let object = try JSONSerialization.jsonObject(with: encoded) as! NSDictionary
            precondition(object == rawCommands[index], "Command changed the fixture: \(object)")
            precondition(encoded.count <= fixtures.bluetooth.minimumCommandBytes, "Canonical command no longer fits BLE minimum payload")
        }
        for (index, reply) in fixtures.replies.enumerated() {
            let encoded = try encoder.encode(reply)
            let decoded = try WireProtocol.decodeReply(encoded)
            precondition(decoded == reply)
            let object = try JSONSerialization.jsonObject(with: encoded) as! NSDictionary
            precondition(object == rawReplies[index], "Reply changed the fixture: \(object)")
        }
        for text in fixtures.validCommandTexts {
            let command = try WireProtocol.decodeCommand(Data(text.utf8))
            let encoded = try encoder.encode(command)
            let decoded = try WireProtocol.decodeCommand(encoded)
            precondition(decoded == command)
        }
        for bad in fixtures.invalidCommands {
            do {
                _ = try WireProtocol.decodeCommand(Data(bad.utf8))
                fatalError("Invalid command accepted: \(bad)")
            } catch {}
        }
        for bad in fixtures.invalidReplies {
            do {
                _ = try WireProtocol.decodeReply(Data(bad.utf8))
                fatalError("Invalid reply accepted: \(bad)")
            } catch {}
        }
        for nonfinite in [Double.nan, .infinity, -.infinity] {
            for command in [Command.move(dx: nonfinite, dy: 0), .scroll(dx: 0, dy: nonfinite)] {
                do {
                    _ = try encoder.encode(command)
                    fatalError("Nonfinite command encoded")
                } catch {}
            }
        }
        precondition(BluetoothProtocol.service == fixtures.bluetooth.service)
        precondition(BluetoothProtocol.command == fixtures.bluetooth.command)
        precondition(BluetoothProtocol.reply == fixtures.bluetooth.reply)
        precondition(BluetoothProtocol.maxFrameBytes == fixtures.bluetooth.maxFrameBytes)
        precondition(BluetoothProtocol.minimumCommandBytes == fixtures.bluetooth.minimumCommandBytes)
        print("PASS: Swift shared wire fixtures, rejected malformed frames, canonical BLE sizes and UUIDs")
    }
}
