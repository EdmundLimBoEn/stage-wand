import CoreBluetooth
import Foundation

@MainActor final class BluetoothServer: NSObject {
    private let session: Session
    private let onCommand: @MainActor @Sendable (Command) -> Void
    private var manager: CBPeripheralManager?
    private var replyCharacteristic: CBMutableCharacteristic?
    private var authed: CBCentral?
    private var pending: [(data: Data, central: CBCentral)] = []

    init(session: Session, onCommand: @escaping @MainActor @Sendable (Command) -> Void) {
        self.session = session
        self.onCommand = onCommand
    }

    func start() {
        guard manager == nil else { return }
        manager = CBPeripheralManager(delegate: self, queue: .main)
    }

    // CoreBluetooth cannot force-disconnect a central; dropping auth is enough because
    // the rotated code makes the phone's re-auth fail with badauth.
    func kick() {
        guard let central = authed else { return }
        authed = nil
        session.peer = nil
        send(.bye(reason: "kicked"), to: central)
    }

    private func send(_ reply: Reply, to central: CBCentral) {
        guard let data = try? JSONEncoder().encode(reply) else { return }
        pending.append((data, central))
        flush()
    }

    private func flush() {
        guard let manager, let characteristic = replyCharacteristic else { return }
        while let next = pending.first {
            guard manager.updateValue(next.data, for: characteristic, onSubscribedCentrals: [next.central]) else { return }
            pending.removeFirst()
        }
    }

    private func handle(_ command: Command, from central: CBCentral) {
        guard central.identifier == authed?.identifier else {
            guard case .auth(let code) = command else { return }
            guard code == session.code else { send(.bye(reason: "badauth"), to: central); return }
            authed = central
            session.peer = "Bluetooth"
            send(.status, to: central)
            return
        }
        switch command {
        case .auth(let code):
            if code == session.code { send(.status, to: central) }
            return
        case .move(let dx, let dy):
            guard dx.isFinite, dy.isFinite, abs(dx) <= 400, abs(dy) <= 400 else { return }
        case .scroll(let dx, let dy):
            guard dx.isFinite, dy.isFinite else { return }
        default: break
        }
        onCommand(command)
    }
}

extension BluetoothServer: @preconcurrency CBPeripheralManagerDelegate {
    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager) {
        guard peripheral.state == .poweredOn else {
            print("Bluetooth not ready: state \(peripheral.state.rawValue)")
            return
        }
        let command = CBMutableCharacteristic(
            type: CBUUID(string: BluetoothProtocol.command),
            properties: [.writeWithoutResponse], value: nil, permissions: [.writeable])
        let reply = CBMutableCharacteristic(
            type: CBUUID(string: BluetoothProtocol.reply),
            properties: [.notify], value: nil, permissions: [.readable])
        let service = CBMutableService(type: CBUUID(string: BluetoothProtocol.service), primary: true)
        service.characteristics = [command, reply]
        replyCharacteristic = reply
        peripheral.removeAllServices()
        peripheral.add(service)
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didAdd service: CBService, error: (any Error)?) {
        if let error {
            print("Bluetooth service failed: \(error)")
            return
        }
        peripheral.startAdvertising([
            CBAdvertisementDataServiceUUIDsKey: [CBUUID(string: BluetoothProtocol.service)],
            CBAdvertisementDataLocalNameKey: Host.current().localizedName ?? "Mac"
        ])
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest]) {
        for request in requests {
            guard let data = request.value,
                  let command = try? JSONDecoder().decode(Command.self, from: data) else { continue }
            handle(command, from: request.central)
        }
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didSubscribeTo characteristic: CBCharacteristic) {
        print("Bluetooth central subscribed: \(central.identifier)")
    }

    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didUnsubscribeFrom characteristic: CBCharacteristic) {
        guard characteristic.uuid == CBUUID(string: BluetoothProtocol.reply),
              central.identifier == authed?.identifier else { return }
        authed = nil
        session.peer = nil
    }

    func peripheralManagerIsReady(toUpdateSubscribers peripheral: CBPeripheralManager) {
        flush()
    }
}
