import Foundation
import CoreBluetooth

enum BluetoothError: Error { case unavailable, disconnected, tooLarge }

@MainActor
final class BluetoothClient: NSObject {
    var onName: ((String) -> Void)?

    private var central: CBCentralManager!
    private var peripheral: CBPeripheral?
    private var commandCharacteristic: CBCharacteristic?
    private var ready = false
    private var wanted = false
    private var inbox: [Data] = []
    private var readyWaiters: [CheckedContinuation<Void, Error>] = []
    private var writeWaiters: [CheckedContinuation<Void, Error>] = []
    private var receiveWaiters: [CheckedContinuation<Data, Error>] = []

    override init() {
        super.init()
        central = CBCentralManager(delegate: self, queue: .main)
    }

    func send(_ data: Data) async throws {
        try await waitReady()
        guard let peripheral, let characteristic = commandCharacteristic else { throw BluetoothError.disconnected }
        guard data.count <= peripheral.maximumWriteValueLength(for: .withoutResponse) else { throw BluetoothError.tooLarge }
        if !peripheral.canSendWriteWithoutResponse {
            try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
                writeWaiters.append(continuation)
            }
        }
        peripheral.writeValue(data, for: characteristic, type: .withoutResponse)
    }

    func receive() async throws -> Data {
        if !inbox.isEmpty { return inbox.removeFirst() }
        return try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Data, Error>) in
            receiveWaiters.append(continuation)
        }
    }

    func cancel() {
        wanted = false
        ready = false
        if central.isScanning { central.stopScan() }
        if let peripheral {
            peripheral.delegate = nil
            central.cancelPeripheralConnection(peripheral)
        }
        peripheral = nil
        commandCharacteristic = nil
        inbox.removeAll()
        failWaiters(CancellationError())
    }

    private func waitReady() async throws {
        if ready { return }
        wanted = true
        switch central.state {
        case .poweredOn: scanIfNeeded()
        case .unauthorized, .poweredOff, .unsupported: throw BluetoothError.unavailable
        default: break
        }
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            readyWaiters.append(continuation)
        }
    }

    private func scanIfNeeded() {
        guard wanted, peripheral == nil, !central.isScanning else { return }
        central.scanForPeripherals(withServices: [CBUUID(string: BluetoothProtocol.service)])
    }

    private func drop(_ error: Error) {
        ready = false
        peripheral?.delegate = nil
        peripheral = nil
        commandCharacteristic = nil
        failWaiters(error)
    }

    private func failWaiters(_ error: Error) {
        let pendingReady = readyWaiters, pendingWrite = writeWaiters, pendingReceive = receiveWaiters
        readyWaiters = []
        writeWaiters = []
        receiveWaiters = []
        for continuation in pendingReady { continuation.resume(throwing: error) }
        for continuation in pendingWrite { continuation.resume(throwing: error) }
        for continuation in pendingReceive { continuation.resume(throwing: error) }
    }
}

extension BluetoothClient: @preconcurrency CBCentralManagerDelegate {
    func centralManagerDidUpdateState(_ central: CBCentralManager) {
        switch central.state {
        case .poweredOn: scanIfNeeded()
        case .unauthorized, .poweredOff, .unsupported: failWaiters(BluetoothError.unavailable)
        default: break
        }
    }

    func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral,
                        advertisementData: [String: Any], rssi RSSI: NSNumber) {
        guard self.peripheral == nil else { return }
        central.stopScan()
        self.peripheral = peripheral
        peripheral.delegate = self
        onName?(advertisementData[CBAdvertisementDataLocalNameKey] as? String ?? peripheral.name ?? "Mac")
        central.connect(peripheral)
    }

    func centralManager(_ central: CBCentralManager, didConnect peripheral: CBPeripheral) {
        peripheral.discoverServices([CBUUID(string: BluetoothProtocol.service)])
    }

    func centralManager(_ central: CBCentralManager, didFailToConnect peripheral: CBPeripheral, error: Error?) {
        drop(BluetoothError.disconnected)
    }

    func centralManager(_ central: CBCentralManager, didDisconnectPeripheral peripheral: CBPeripheral, error: Error?) {
        drop(BluetoothError.disconnected)
    }
}

extension BluetoothClient: @preconcurrency CBPeripheralDelegate {
    func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?) {
        guard let service = peripheral.services?.first(where: { $0.uuid == CBUUID(string: BluetoothProtocol.service) }) else {
            drop(BluetoothError.disconnected)
            return
        }
        peripheral.discoverCharacteristics([CBUUID(string: BluetoothProtocol.command),
                                           CBUUID(string: BluetoothProtocol.reply)], for: service)
    }

    func peripheral(_ peripheral: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?) {
        for characteristic in service.characteristics ?? [] {
            switch characteristic.uuid.uuidString.uppercased() {
            case BluetoothProtocol.command: commandCharacteristic = characteristic
            case BluetoothProtocol.reply: peripheral.setNotifyValue(true, for: characteristic)
            default: break
            }
        }
    }

    func peripheral(_ peripheral: CBPeripheral, didUpdateNotificationStateFor characteristic: CBCharacteristic, error: Error?) {
        guard characteristic.isNotifying, commandCharacteristic != nil else { return }
        ready = true
        let waiters = readyWaiters
        readyWaiters = []
        for continuation in waiters { continuation.resume() }
    }

    func peripheral(_ peripheral: CBPeripheral, didUpdateValueFor characteristic: CBCharacteristic, error: Error?) {
        guard let value = characteristic.value, !value.isEmpty else { return }
        if receiveWaiters.isEmpty { inbox.append(value) }
        else { receiveWaiters.removeFirst().resume(returning: value) }
    }

    func peripheralIsReady(toSendWriteWithoutResponse peripheral: CBPeripheral) {
        let waiters = writeWaiters
        writeWaiters = []
        for continuation in waiters { continuation.resume() }
    }
}
