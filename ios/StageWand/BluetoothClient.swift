import Foundation
import CoreBluetooth

enum BluetoothError: Error, LocalizedError {
    case unavailable, disconnected, tooLarge, incompatible

    var errorDescription: String? {
        switch self {
        case .unavailable: "Bluetooth unavailable. Enable Bluetooth and allow Stage Wand in Settings."
        case .disconnected: "Bluetooth disconnected."
        case .tooLarge: "This Bluetooth connection cannot carry Stage Wand commands. Try Wi-Fi nearby."
        case .incompatible: "This device does not provide a compatible Stage Wand Bluetooth service."
        }
    }
}

@MainActor
final class BluetoothClient: NSObject {
    var onName: ((String) -> Void)?

    private var central: CBCentralManager?
    private var peripheral: CBPeripheral?
    private var commandCharacteristic: CBCharacteristic?
    private var replyCharacteristic: CBCharacteristic?
    private var ready = false
    private var wanted = false
    private var generation = UUID()
    private var terminalError: Error = BluetoothError.disconnected
    private var inbox: [Data] = []
    private var readyWaiters: [UUID: CheckedContinuation<Void, Error>] = [:]
    private var writeWaiters: [UUID: CheckedContinuation<Void, Error>] = [:]
    private var receiveWaiters: [(id: UUID, continuation: CheckedContinuation<Data, Error>)] = []

    func start() {
        cancel()
        wanted = true
        central = CBCentralManager(delegate: self, queue: .main)
        updateState()
    }

    func send(_ data: Data) async throws {
        try Task.checkCancellation()
        let id = generation
        try await waitReady()
        guard wanted, generation == id, let peripheral, let characteristic = commandCharacteristic else {
            throw terminalError
        }
        guard data.count <= BluetoothProtocol.maxFrameBytes,
              data.count <= peripheral.maximumWriteValueLength(for: .withoutResponse) else {
            throw BluetoothError.tooLarge
        }
        while !peripheral.canSendWriteWithoutResponse {
            let waiterID = UUID()
            let timeout = Task { @MainActor [weak self] in
                do { try await Task.sleep(for: .seconds(5)) } catch { return }
                guard let self, self.generation == id, self.writeWaiters[waiterID] != nil else { return }
                self.drop(BluetoothError.disconnected)
            }
            defer { timeout.cancel() }
            try await withTaskCancellationHandler {
                try Task.checkCancellation()
                try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                    writeWaiters[waiterID] = continuation
                }
            } onCancel: {
                Task { @MainActor [weak self] in
                    self?.writeWaiters.removeValue(forKey: waiterID)?.resume(throwing: CancellationError())
                }
            }
            guard wanted, generation == id, self.peripheral === peripheral, ready else { throw terminalError }
        }
        try Task.checkCancellation()
        peripheral.writeValue(data, for: characteristic, type: .withoutResponse)
    }

    func receive() async throws -> Data {
        try Task.checkCancellation()
        guard wanted else { throw terminalError }
        if !inbox.isEmpty { return inbox.removeFirst() }
        let id = UUID()
        return try await withTaskCancellationHandler {
            try Task.checkCancellation()
            return try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Data, Error>) in
                receiveWaiters.append((id, continuation))
            }
        } onCancel: {
            Task { @MainActor [weak self] in
                guard let self, let index = self.receiveWaiters.firstIndex(where: { $0.id == id }) else { return }
                self.receiveWaiters.remove(at: index).continuation.resume(throwing: CancellationError())
            }
        }
    }

    func cancel() { drop(CancellationError()) }

    private func waitReady() async throws {
        try Task.checkCancellation()
        guard wanted else { throw terminalError }
        if ready { return }
        let id = UUID()
        try await withTaskCancellationHandler {
            try Task.checkCancellation()
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                readyWaiters[id] = continuation
            }
        } onCancel: {
            Task { @MainActor [weak self] in
                self?.readyWaiters.removeValue(forKey: id)?.resume(throwing: CancellationError())
            }
        }
    }

    private func scanIfNeeded() {
        guard wanted, peripheral == nil, let central, central.state == .poweredOn, !central.isScanning else { return }
        central.scanForPeripherals(withServices: [CBUUID(string: BluetoothProtocol.service)])
    }

    private func updateState() {
        guard wanted, let central else { return }
        switch central.state {
        case .poweredOn: scanIfNeeded()
        case .unauthorized, .poweredOff, .unsupported: drop(BluetoothError.unavailable)
        case .resetting: drop(BluetoothError.disconnected)
        case .unknown: break
        @unknown default: drop(BluetoothError.unavailable)
        }
    }

    private func drop(_ error: Error) {
        wanted = false
        ready = false
        generation = UUID()
        terminalError = error
        if central?.isScanning == true { central?.stopScan() }
        if let peripheral {
            peripheral.delegate = nil
            central?.cancelPeripheralConnection(peripheral)
        }
        central?.delegate = nil
        central = nil
        peripheral = nil
        commandCharacteristic = nil
        replyCharacteristic = nil
        inbox.removeAll()
        let pendingReady = readyWaiters, pendingWrite = writeWaiters, pendingReceive = receiveWaiters
        readyWaiters.removeAll()
        writeWaiters.removeAll()
        receiveWaiters.removeAll()
        for continuation in pendingReady.values { continuation.resume(throwing: error) }
        for continuation in pendingWrite.values { continuation.resume(throwing: error) }
        for waiter in pendingReceive { waiter.continuation.resume(throwing: error) }
    }
}

extension BluetoothClient: @preconcurrency CBCentralManagerDelegate {
    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        guard self.central === central else { return }
        updateState()
    }

    func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral,
                        advertisementData: [String: Any], rssi RSSI: NSNumber) {
        guard self.central === central, wanted, self.peripheral == nil else { return }
        central.stopScan()
        self.peripheral = peripheral
        peripheral.delegate = self
        onName?(advertisementData[CBAdvertisementDataLocalNameKey] as? String ?? peripheral.name ?? "Computer")
        central.connect(peripheral)
    }

    func centralManager(_ central: CBCentralManager, didConnect peripheral: CBPeripheral) {
        guard self.central === central, wanted, self.peripheral === peripheral else { return }
        peripheral.discoverServices([CBUUID(string: BluetoothProtocol.service)])
    }

    func centralManager(_ central: CBCentralManager, didFailToConnect peripheral: CBPeripheral, error: Error?) {
        guard self.central === central, self.peripheral === peripheral else { return }
        drop(BluetoothError.disconnected)
    }

    func centralManager(_ central: CBCentralManager, didDisconnectPeripheral peripheral: CBPeripheral, error: Error?) {
        guard self.central === central, self.peripheral === peripheral else { return }
        drop(BluetoothError.disconnected)
    }
}

extension BluetoothClient: @preconcurrency CBPeripheralDelegate {
    func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?) {
        guard wanted, self.peripheral === peripheral else { return }
        guard error == nil else { drop(BluetoothError.disconnected); return }
        guard let service = peripheral.services?.first(where: { $0.uuid == CBUUID(string: BluetoothProtocol.service) }) else {
            drop(BluetoothError.incompatible)
            return
        }
        peripheral.discoverCharacteristics([CBUUID(string: BluetoothProtocol.command),
                                           CBUUID(string: BluetoothProtocol.reply)], for: service)
    }

    func peripheral(_ peripheral: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        guard wanted, self.peripheral === peripheral, service.uuid == CBUUID(string: BluetoothProtocol.service) else { return }
        guard error == nil else { drop(BluetoothError.disconnected); return }
        guard let command = service.characteristics?.first(where: { $0.uuid == CBUUID(string: BluetoothProtocol.command) }),
              command.properties.contains(.writeWithoutResponse),
              let reply = service.characteristics?.first(where: { $0.uuid == CBUUID(string: BluetoothProtocol.reply) }),
              reply.properties.contains(.notify) else {
            drop(BluetoothError.incompatible)
            return
        }
        guard peripheral.maximumWriteValueLength(for: .withoutResponse) >= BluetoothProtocol.minimumCommandBytes else {
            drop(BluetoothError.tooLarge)
            return
        }
        commandCharacteristic = command
        replyCharacteristic = reply
        peripheral.setNotifyValue(true, for: reply)
    }

    func peripheral(_ peripheral: CBPeripheral, didUpdateNotificationStateFor characteristic: CBCharacteristic, error: Error?) {
        guard wanted, self.peripheral === peripheral, characteristic === replyCharacteristic else { return }
        guard error == nil, characteristic.isNotifying, commandCharacteristic != nil else {
            drop(BluetoothError.disconnected)
            return
        }
        ready = true
        let waiters = readyWaiters
        readyWaiters.removeAll()
        for continuation in waiters.values { continuation.resume() }
    }

    func peripheral(_ peripheral: CBPeripheral, didUpdateValueFor characteristic: CBCharacteristic, error: Error?) {
        guard wanted, ready, self.peripheral === peripheral, characteristic === replyCharacteristic else { return }
        guard error == nil else { drop(BluetoothError.disconnected); return }
        guard let value = characteristic.value, !value.isEmpty, value.count <= BluetoothProtocol.maxFrameBytes else {
            drop(BluetoothError.incompatible)
            return
        }
        if !receiveWaiters.isEmpty { receiveWaiters.removeFirst().continuation.resume(returning: value) }
        else if inbox.count < 16 { inbox.append(value) }
        else { drop(BluetoothError.disconnected) }
    }

    func peripheralIsReady(toSendWriteWithoutResponse peripheral: CBPeripheral) {
        guard wanted, ready, self.peripheral === peripheral else { return }
        let waiters = writeWaiters
        writeWaiters.removeAll()
        for continuation in waiters.values { continuation.resume() }
    }

    func peripheral(_ peripheral: CBPeripheral, didModifyServices invalidatedServices: [CBService]) {
        guard wanted, self.peripheral === peripheral,
              invalidatedServices.contains(where: { $0.uuid == CBUUID(string: BluetoothProtocol.service) }) else { return }
        drop(BluetoothError.disconnected)
    }
}
