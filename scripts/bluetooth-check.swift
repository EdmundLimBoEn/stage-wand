import Foundation
import CoreBluetooth

@MainActor
final class Session {
    let code = "4567"
    var bluetoothError: String?
    private(set) var peer: String?
    private var authentication = AuthenticationLimiter()
    private let ownership = PeerOwnership()

    func authorize(_ candidate: String) -> Bool { authentication.authorize(candidate, expected: code) }
    func claimPeer(id: UUID, name: String, onDisplaced: @escaping @MainActor () -> Void) {
        ownership.claim(id: id, onDisplaced: onDisplaced)
        peer = name
    }
    func releasePeer(id: UUID) { if ownership.release(id: id) { peer = nil } }
    func isActivePeer(id: UUID) -> Bool { ownership.contains(id: id) }
}

@MainActor
final class Commands {
    var values: [Command] = []
}

@main
struct BluetoothCheck {
    @MainActor static func settle() async {
        for _ in 0..<12 { await Task.yield() }
        try? await Task.sleep(for: .milliseconds(10))
    }

    @MainActor static func ready(_ client: BluetoothClient, capacity: Int = 185,
                                 notifyError: Error? = nil, device: CBPeripheral? = nil) -> (CBCentralManager, CBPeripheral, CBCharacteristic) {
        let manager = CBCentralManager.latest!
        let peripheral = device ?? CBPeripheral()
        peripheral.capacity = capacity
        let command = CBCharacteristic(type: CBUUID(string: BluetoothProtocol.command), properties: [.writeWithoutResponse], value: nil)
        let reply = CBCharacteristic(type: CBUUID(string: BluetoothProtocol.reply), properties: [.notify], value: nil)
        let service = CBService(type: CBUUID(string: BluetoothProtocol.service))
        service.characteristics = [command, reply]
        peripheral.services = [service]
        client.centralManager(manager, didDiscover: peripheral, advertisementData: [:], rssi: -40)
        client.centralManager(manager, didConnect: peripheral)
        client.peripheral(peripheral, didDiscoverServices: nil)
        client.peripheral(peripheral, didDiscoverCharacteristicsFor: service, error: nil)
        reply.isNotifying = notifyError == nil
        client.peripheral(peripheral, didUpdateNotificationStateFor: reply, error: notifyError)
        return (manager, peripheral, reply)
    }

    @MainActor static func assertFailure<T>(_ task: Task<T, Error>, _ label: String) async {
        do { _ = try await task.value; fatalError("Expected failure: \(label)") }
        catch { }
    }

    @MainActor static func clientChecks() async throws {
        precondition(CBCentralManager.latest == nil)
        let client = BluetoothClient()
        precondition(CBCentralManager.latest == nil, "Initializing Wi-Fi-only Link must not prompt for Bluetooth")
        client.start()
        let pending = Task { try await client.send(Data("{}".utf8)) }
        await settle()
        pending.cancel()
        await assertFailure(pending, "cancel connection wait")
        let (manager, peripheral, reply) = ready(client)
        let command = try JSONEncoder().encode(Command.auth(code: "4567"))
        try await client.send(command)
        precondition(peripheral.writes == [command])

        let receiver = Task { try await client.receive() }
        await settle()
        receiver.cancel()
        await assertFailure(receiver, "cancel pending receive")
        reply.value = try JSONEncoder().encode(Reply.status)
        client.peripheral(peripheral, didUpdateValueFor: reply, error: nil)
        let received = try await client.receive()
        precondition(received == reply.value)

        peripheral.canSendWriteWithoutResponse = false
        let blocked = Task { try await client.send(command) }
        await settle()
        precondition(peripheral.writes.count == 1)
        blocked.cancel()
        await assertFailure(blocked, "cancel congested write")
        peripheral.canSendWriteWithoutResponse = true
        client.peripheralIsReady(toSendWriteWithoutResponse: peripheral)
        await settle()
        precondition(peripheral.writes.count == 1, "Cancelled write leaked onto radio")

        peripheral.canSendWriteWithoutResponse = false
        let first = Task { try await client.send(command) }
        let second = Task { try await client.send(command) }
        await settle()
        peripheral.onWrite = { _ in peripheral.canSendWriteWithoutResponse = false }
        peripheral.canSendWriteWithoutResponse = true
        client.peripheralIsReady(toSendWriteWithoutResponse: peripheral)
        await settle()
        precondition(peripheral.writes.count == 2, "Resumed senders ignored exhausted write credit")
        peripheral.canSendWriteWithoutResponse = true
        client.peripheralIsReady(toSendWriteWithoutResponse: peripheral)
        try await first.value
        try await second.value
        precondition(peripheral.writes.count == 3)
        peripheral.onWrite = nil

        let waiting = Task { try await client.receive() }
        let writing = Task { try await client.send(command) }
        await settle()
        manager.state = .poweredOff
        client.centralManagerDidUpdateState(manager)
        await assertFailure(waiting, "power off receive")
        await assertFailure(writing, "power off write")
        precondition(peripheral.delegate == nil)
        manager.state = .poweredOn
        client.start()
        let (newManager, replacement, nextReply) = ready(client, device: peripheral)
        precondition(newManager !== manager, "Connection attempts share a callback source")
        replacement.canSendWriteWithoutResponse = true
        let replacementWriteCount = replacement.writes.count
        client.centralManager(manager, didDisconnectPeripheral: peripheral, error: nil)
        client.peripheral(peripheral, didUpdateValueFor: reply, error: nil)
        try await client.send(command)
        precondition(replacement.writes.count == replacementWriteCount + 1, "Old disconnect destroyed replacement link")
        nextReply.value = Data("{\"t\":\"status\"}".utf8)
        client.peripheral(replacement, didUpdateValueFor: nextReply, error: nil)
        let next = try await client.receive()
        precondition(next == nextReply.value)
        await assertFailure(Task { try await client.send(Data(repeating: 0, count: 513)) }, "oversized outbound frame")

        for _ in 0..<17 { client.peripheral(replacement, didUpdateValueFor: nextReply, error: nil) }
        await assertFailure(Task { try await client.receive() }, "bounded incoming queue")
        client.start()
        _ = ready(client, capacity: 20)
        do { try await client.send(command); fatalError("Insufficient ATT payload accepted") }
        catch BluetoothError.tooLarge { }
        client.start()
        _ = ready(client, notifyError: BluetoothError.disconnected)
        await assertFailure(Task { try await client.send(command) }, "notification subscription failure")
        client.start()
        let (_, invalidated, _) = ready(client)
        client.peripheral(invalidated, didModifyServices: invalidated.services!)
        await assertFailure(Task { try await client.send(command) }, "GATT service invalidation")
        client.start()
        let (_, stalled, _) = ready(client)
        stalled.canSendWriteWithoutResponse = false
        let stalledRead = Task { try await client.receive() }
        await assertFailure(Task { try await client.send(command) }, "stalled write timeout")
        await assertFailure(stalledRead, "stalled write cancelled receive")
        client.cancel()
        client.centralManager(manager, didDiscover: CBPeripheral(), advertisementData: [:], rssi: -40)
        precondition(!manager.isScanning, "Cancelled attempt resumed scanning")
    }

    @MainActor static func serverChecks() throws {
        let session = Session()
        let commands = Commands()
        let server = BluetoothServer(session: session) { commands.values.append($0) }
        server.start()
        let manager = CBPeripheralManager.latest!
        server.peripheralManagerDidUpdateState(manager)
        let service = manager.services[0]
        server.peripheralManager(manager, didAdd: service, error: nil)
        precondition(manager.isAdvertising)
        let command = service.characteristics![0], reply = service.characteristics![1]
        let first = CBCentral(), second = CBCentral()
        func write(_ value: Command, from central: CBCentral) throws {
            server.peripheralManager(manager, didReceiveWrite: [CBATTRequest(characteristic: command, central: central,
                                                                           value: try JSONEncoder().encode(value))])
        }
        try write(.auth(code: session.code), from: first)
        precondition(session.peer == nil, "Authenticated before reply subscription")
        server.peripheralManager(manager, central: first, didSubscribeTo: reply)
        try write(.auth(code: session.code), from: first)
        precondition(session.isActivePeer(id: first.identifier))
        let firstReply = try JSONDecoder().decode(Reply.self, from: manager.notifications.last!.0)
        precondition(firstReply == .status && manager.notifications.last!.1 === first)
        try write(.key(.right), from: first)
        precondition(commands.values == [.key(.right)])
        try write(.scroll(dx: 401, dy: 0), from: first)
        precondition(commands.values.count == 1, "Out-of-range scroll dispatched")

        let wrongOffset = CBATTRequest(characteristic: command, central: first, value: try JSONEncoder().encode(Command.key(.left)))
        wrongOffset.offset = 1
        let valid = CBATTRequest(characteristic: command, central: first, value: try JSONEncoder().encode(Command.key(.left)))
        server.peripheralManager(manager, didReceiveWrite: [valid, wrongOffset])
        precondition(commands.values.count == 1, "Partially invalid write batch was partially dispatched")
        precondition(manager.responses.last == .invalidOffset)
        let duplicate = CBATTRequest(characteristic: command, central: first,
                                     value: Data("{\"t\":\"key\",\"t\":\"click\",\"k\":\"left\",\"b\":\"left\"}".utf8))
        server.peripheralManager(manager, didReceiveWrite: [duplicate])
        precondition(manager.responses.last == .invalidAttributeValueLength && commands.values.count == 1,
                     "Duplicate command keys reached input dispatch")
        let oversized = CBATTRequest(characteristic: command, central: first, value: Data(repeating: 32, count: 513))
        server.peripheralManager(manager, didReceiveWrite: [oversized])
        precondition(manager.responses.last == .invalidAttributeValueLength)

        server.peripheralManager(manager, central: second, didSubscribeTo: reply)
        try write(.auth(code: session.code), from: second)
        precondition(session.isActivePeer(id: second.identifier))
        let displaced = try JSONDecoder().decode(Reply.self, from: manager.notifications[manager.notifications.count - 2].0)
        precondition(displaced == .bye(reason: "displaced"))
        try write(.key(.left), from: first)
        try write(.key(.left), from: second)
        precondition(commands.values == [.key(.right), .key(.left)])
        server.peripheralManager(manager, central: first, didUnsubscribeFrom: reply)
        precondition(session.isActivePeer(id: second.identifier), "Old unsubscribe cleared new owner")

        let lan = UUID()
        session.claimPeer(id: lan, name: "LAN") { }
        try write(.key(.right), from: second)
        precondition(commands.values.count == 2 && session.isActivePeer(id: lan), "BLE retained input after LAN displacement")
        try write(.auth(code: session.code), from: second)
        precondition(session.isActivePeer(id: second.identifier))
        server.peripheralManager(manager, central: second, didSubscribeTo: reply)
        precondition(session.peer == nil, "New subscription inherited old authentication")
        try write(.auth(code: session.code), from: second)

        second.maximumUpdateValueLength = 20
        server.peripheralManager(manager, central: second, didSubscribeTo: reply)
        try write(.auth(code: session.code), from: second)
        precondition(session.peer == nil, "Insufficient payload subscription retained old authentication")
        second.maximumUpdateValueLength = 185
        server.peripheralManager(manager, central: second, didSubscribeTo: reply)
        try write(.auth(code: session.code), from: second)

        manager.hasCapacity = false
        let notificationCount = manager.notifications.count
        for _ in 0..<100 { try write(.auth(code: session.code), from: second) }
        server.kick()
        manager.hasCapacity = true
        server.peripheralManagerIsReady(toUpdateSubscribers: manager)
        precondition(manager.notifications.count == notificationCount + 1, "Unbounded or obsolete notifications replayed")
        let kicked = try JSONDecoder().decode(Reply.self, from: manager.notifications.last!.0)
        precondition(kicked == .bye(reason: "kicked"))

        for _ in 0..<5 { try write(.auth(code: "0000"), from: second) }
        try write(.auth(code: session.code), from: second)
        precondition(session.peer == nil, "BLE bypassed global pairing throttle")
        manager.state = .poweredOff
        server.peripheralManagerDidUpdateState(manager)
        precondition(session.peer == nil && session.bluetoothError != nil)
        manager.state = .poweredOn
        server.peripheralManagerDidUpdateState(manager)
        server.peripheralManager(manager, didAdd: service, error: nil)
        precondition(session.bluetoothError == nil)
        server.peripheralManager(manager, didReceiveWrite: [valid])
        precondition(commands.values.count == 2, "Radio restart retained authenticated central")
    }

    @MainActor static func main() async throws {
        let watchdog = Task {
            try await Task.sleep(for: .seconds(20))
            fatalError("Bluetooth transport checks timed out")
        }
        defer { watchdog.cancel() }
        try await clientChecks()
        try serverChecks()
        print("Bluetooth checks passed: production Apple lifecycle, cancellation, MTU, backpressure, bounded queues, stale callbacks, subscription, auth throttle, ownership, malformed writes and radio restart")
    }
}
