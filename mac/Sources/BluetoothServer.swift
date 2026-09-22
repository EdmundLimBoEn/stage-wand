import CoreBluetooth
import Foundation

@MainActor final class BluetoothServer: NSObject {
    private let session: Session
    private let onCommand: @MainActor @Sendable (Command) -> Void
    private var manager: CBPeripheralManager?
    private var service: CBMutableService?
    private var replyCharacteristic: CBMutableCharacteristic?
    private var authed: CBCentral?
    private var subscribers: [UUID: CBCentral] = [:]
    private var pending: [(data: Data, central: CBCentral)] = []

    init(session: Session, onCommand: @escaping @MainActor @Sendable (Command) -> Void) {
        self.session = session
        self.onCommand = onCommand
        super.init()
    }

    func start() {
        guard manager == nil else { return }
        manager = CBPeripheralManager(delegate: self, queue: .main)
    }

    func kick() { close(reason: "kicked") }

    private func close(reason: String? = nil) {
        guard let central = authed else { return }
        authed = nil
        session.releasePeer(id: central.identifier)
        pending.removeAll { $0.central.identifier == central.identifier }
        if let reason { send(.bye(reason: reason), to: central) }
    }

    private func reset() {
        close()
        subscribers.removeAll()
        pending.removeAll()
        replyCharacteristic = nil
        service = nil
    }

    private func send(_ reply: Reply, to central: CBCentral) {
        guard subscribers[central.identifier] != nil,
              let data = try? JSONEncoder().encode(reply),
              data.count <= BluetoothProtocol.maxFrameBytes,
              data.count <= central.maximumUpdateValueLength else { return }
        // Only the latest session state matters while the controller's notification buffer is full.
        pending.removeAll { $0.central.identifier == central.identifier }
        pending.append((data, central))
        flush()
    }

    private func flush() {
        guard let manager, manager.state == .poweredOn, let characteristic = replyCharacteristic else { return }
        while let next = pending.first {
            guard subscribers[next.central.identifier] != nil else {
                pending.removeFirst()
                continue
            }
            guard manager.updateValue(next.data, for: characteristic, onSubscribedCentrals: [next.central]) else { return }
            pending.removeFirst()
        }
    }

    private func handle(_ command: Command, from central: CBCentral) {
        guard subscribers[central.identifier] != nil else { return }
        if case .auth(let code) = command {
            guard session.authorize(code) else {
                if authed?.identifier == central.identifier { close() }
                send(.bye(reason: "badauth"), to: central)
                return
            }
            if authed?.identifier != central.identifier { close(reason: "displaced") }
            authed = central
            session.claimPeer(id: central.identifier, name: "Bluetooth") { [weak self] in
                self?.close(reason: "displaced")
            }
            send(.status, to: central)
            return
        }
        guard central.identifier == authed?.identifier, session.isActivePeer(id: central.identifier) else { return }
        switch command {
        case .move(let dx, let dy):
            guard dx.isFinite, dy.isFinite, abs(dx) <= 400, abs(dy) <= 400 else { return }
        case .scroll(let dx, let dy):
            guard dx.isFinite, dy.isFinite, abs(dx) <= 400, abs(dy) <= 400 else { return }
        default: break
        }
        onCommand(command)
    }
}

extension BluetoothServer: @preconcurrency CBPeripheralManagerDelegate {
    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager) {
        guard manager === peripheral else { return }
        reset()
        guard peripheral.state == .poweredOn else {
            session.bluetoothError = "Bluetooth unavailable. Enable Bluetooth and allow Stage Wand in System Settings."
            return
        }
        session.bluetoothError = nil
        let command = CBMutableCharacteristic(
            type: CBUUID(string: BluetoothProtocol.command),
            properties: [.writeWithoutResponse, .write], value: nil, permissions: [.writeable])
        let reply = CBMutableCharacteristic(
            type: CBUUID(string: BluetoothProtocol.reply),
            properties: [.notify], value: nil, permissions: [.readable])
        let service = CBMutableService(type: CBUUID(string: BluetoothProtocol.service), primary: true)
        service.characteristics = [command, reply]
        self.service = service
        replyCharacteristic = reply
        peripheral.stopAdvertising()
        peripheral.removeAllServices()
        peripheral.add(service)
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didAdd service: CBService, error: (any Error)?) {
        guard manager === peripheral, self.service === service, peripheral.state == .poweredOn else { return }
        if let error {
            session.bluetoothError = "Bluetooth service failed: \(error.localizedDescription)"
            return
        }
        peripheral.startAdvertising([
            CBAdvertisementDataServiceUUIDsKey: [CBUUID(string: BluetoothProtocol.service)],
            CBAdvertisementDataLocalNameKey: Host.current().localizedName ?? "Mac"
        ])
    }

    func peripheralManagerDidStartAdvertising(_ peripheral: CBPeripheralManager, error: Error?) {
        guard manager === peripheral, peripheral.state == .poweredOn else { return }
        session.bluetoothError = error.map { "Bluetooth advertising failed: \($0.localizedDescription)" }
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest]) {
        guard manager === peripheral, peripheral.state == .poweredOn, let first = requests.first else { return }
        guard requests.count <= 32 else { peripheral.respond(to: first, withResult: .insufficientResources); return }
        var commands: [(Command, CBCentral)] = []
        for request in requests {
            guard request.characteristic.uuid == CBUUID(string: BluetoothProtocol.command) else {
                peripheral.respond(to: first, withResult: .writeNotPermitted)
                return
            }
            guard request.offset == 0 else { peripheral.respond(to: first, withResult: .invalidOffset); return }
            guard subscribers[request.central.identifier] != nil else {
                peripheral.respond(to: first, withResult: .insufficientAuthentication)
                return
            }
            guard let data = request.value, !data.isEmpty, data.count <= BluetoothProtocol.maxFrameBytes,
                  let command = try? WireProtocol.decodeCommand(data) else {
                peripheral.respond(to: first, withResult: .invalidAttributeValueLength)
                return
            }
            commands.append((command, request.central))
        }
        for (command, central) in commands { handle(command, from: central) }
        peripheral.respond(to: first, withResult: .success)
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didSubscribeTo characteristic: CBCharacteristic) {
        guard manager === peripheral, peripheral.state == .poweredOn, characteristic === replyCharacteristic else { return }
        if central.identifier == authed?.identifier { close() }
        subscribers.removeValue(forKey: central.identifier)
        pending.removeAll { $0.central.identifier == central.identifier }
        guard central.maximumUpdateValueLength >= BluetoothProtocol.minimumCommandBytes,
              subscribers.count < 16 else { return }
        subscribers[central.identifier] = central
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didUnsubscribeFrom characteristic: CBCharacteristic) {
        guard manager === peripheral, characteristic === replyCharacteristic else { return }
        subscribers.removeValue(forKey: central.identifier)
        pending.removeAll { $0.central.identifier == central.identifier }
        if central.identifier == authed?.identifier { close() }
        flush()
    }

    func peripheralManagerIsReady(toUpdateSubscribers peripheral: CBPeripheralManager) {
        guard manager === peripheral else { return }
        flush()
    }
}
