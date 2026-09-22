// Deterministic radio boundary for exercising the production transports without Bluetooth hardware.
import Foundation

public let CBAdvertisementDataLocalNameKey = "localName"
public let CBAdvertisementDataServiceUUIDsKey = "serviceUUIDs"

public enum CBManagerState: Int { case unknown, resetting, unsupported, unauthorized, poweredOff, poweredOn }
public enum CBCharacteristicWriteType { case withResponse, withoutResponse }
public enum CBATTError {
    public enum Code { case success, writeNotPermitted, invalidOffset, insufficientAuthentication, invalidAttributeValueLength, insufficientResources }
}
public struct CBUUID: Equatable, Sendable {
    public let uuidString: String
    public init(string: String) { uuidString = string.uppercased() }
}
public struct CBCharacteristicProperties: OptionSet, Sendable {
    public let rawValue: Int
    public init(rawValue: Int) { self.rawValue = rawValue }
    public static let write = Self(rawValue: 1)
    public static let writeWithoutResponse = Self(rawValue: 2)
    public static let notify = Self(rawValue: 4)
}
public struct CBAttributePermissions: OptionSet, Sendable {
    public let rawValue: Int
    public init(rawValue: Int) { self.rawValue = rawValue }
    public static let writeable = Self(rawValue: 1)
    public static let readable = Self(rawValue: 2)
}

public protocol CBCentralManagerDelegate: AnyObject {
    func centralManagerDidUpdateState(_ central: CBCentralManager)
    func centralManager(_ central: CBCentralManager, didDiscover peripheral: CBPeripheral, advertisementData: [String: Any], rssi RSSI: NSNumber)
    func centralManager(_ central: CBCentralManager, didConnect peripheral: CBPeripheral)
    func centralManager(_ central: CBCentralManager, didFailToConnect peripheral: CBPeripheral, error: Error?)
    func centralManager(_ central: CBCentralManager, didDisconnectPeripheral peripheral: CBPeripheral, error: Error?)
}
public protocol CBPeripheralDelegate: AnyObject {
    func peripheral(_ peripheral: CBPeripheral, didDiscoverServices error: Error?)
    func peripheral(_ peripheral: CBPeripheral, didDiscoverCharacteristicsFor service: CBService, error: Error?)
    func peripheral(_ peripheral: CBPeripheral, didUpdateNotificationStateFor characteristic: CBCharacteristic, error: Error?)
    func peripheral(_ peripheral: CBPeripheral, didUpdateValueFor characteristic: CBCharacteristic, error: Error?)
    func peripheralIsReady(toSendWriteWithoutResponse peripheral: CBPeripheral)
    func peripheral(_ peripheral: CBPeripheral, didModifyServices invalidatedServices: [CBService])
}
public protocol CBPeripheralManagerDelegate: AnyObject {
    func peripheralManagerDidUpdateState(_ peripheral: CBPeripheralManager)
    func peripheralManager(_ peripheral: CBPeripheralManager, didAdd service: CBService, error: Error?)
    func peripheralManagerDidStartAdvertising(_ peripheral: CBPeripheralManager, error: Error?)
    func peripheralManager(_ peripheral: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest])
    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didSubscribeTo characteristic: CBCharacteristic)
    func peripheralManager(_ peripheral: CBPeripheralManager, central: CBCentral, didUnsubscribeFrom characteristic: CBCharacteristic)
    func peripheralManagerIsReady(toUpdateSubscribers peripheral: CBPeripheralManager)
}

@MainActor public class CBCharacteristic: NSObject {
    public let uuid: CBUUID
    public let properties: CBCharacteristicProperties
    public var value: Data?
    public var isNotifying = false
    public init(type: CBUUID, properties: CBCharacteristicProperties, value: Data?) {
        uuid = type
        self.properties = properties
        self.value = value
    }
}
@MainActor public final class CBMutableCharacteristic: CBCharacteristic {
    public init(type: CBUUID, properties: CBCharacteristicProperties, value: Data?, permissions: CBAttributePermissions) {
        super.init(type: type, properties: properties, value: value)
    }
}
@MainActor public class CBService: NSObject {
    public let uuid: CBUUID
    public var characteristics: [CBCharacteristic]?
    public init(type: CBUUID) { uuid = type }
}
@MainActor public final class CBMutableService: CBService {
    public init(type: CBUUID, primary: Bool) { super.init(type: type) }
}
@MainActor public final class CBPeripheral: NSObject {
    public weak var delegate: CBPeripheralDelegate?
    public var name: String? = "Test host"
    public var services: [CBService]?
    public var canSendWriteWithoutResponse = true
    public var capacity = 185
    public var writes: [Data] = []
    public var subscribed: CBCharacteristic?
    public var onWrite: ((Data) -> Void)?
    public var discoveredServices = false
    public func discoverServices(_ ids: [CBUUID]?) { discoveredServices = true }
    public func discoverCharacteristics(_ ids: [CBUUID]?, for service: CBService) {}
    public func setNotifyValue(_ value: Bool, for characteristic: CBCharacteristic) { subscribed = characteristic }
    public func maximumWriteValueLength(for type: CBCharacteristicWriteType) -> Int { capacity }
    public func writeValue(_ data: Data, for characteristic: CBCharacteristic, type: CBCharacteristicWriteType) {
        precondition(canSendWriteWithoutResponse, "Write without available credit")
        precondition(data.count <= capacity, "Oversized ATT write")
        writes.append(data)
        onWrite?(data)
    }
}
@MainActor public final class CBCentralManager: NSObject {
    public static var latest: CBCentralManager?
    public weak var delegate: CBCentralManagerDelegate?
    public var state = CBManagerState.poweredOn
    public var isScanning = false
    public var connections: [CBPeripheral] = []
    public var cancelled: [CBPeripheral] = []
    public init(delegate: CBCentralManagerDelegate?, queue: DispatchQueue?) {
        self.delegate = delegate
        super.init()
        Self.latest = self
    }
    public func scanForPeripherals(withServices: [CBUUID]?) { isScanning = true }
    public func stopScan() { isScanning = false }
    public func connect(_ peripheral: CBPeripheral) { connections.append(peripheral) }
    public func cancelPeripheralConnection(_ peripheral: CBPeripheral) { cancelled.append(peripheral) }
}
@MainActor public final class CBCentral: NSObject {
    public let identifier = UUID()
    public var maximumUpdateValueLength = 185
}
@MainActor public final class CBATTRequest: NSObject {
    public let characteristic: CBCharacteristic
    public let central: CBCentral
    public var value: Data?
    public var offset = 0
    public init(characteristic: CBCharacteristic, central: CBCentral, value: Data?) {
        self.characteristic = characteristic
        self.central = central
        self.value = value
    }
}
@MainActor public final class CBPeripheralManager: NSObject {
    public static var latest: CBPeripheralManager?
    public weak var delegate: CBPeripheralManagerDelegate?
    public var state = CBManagerState.poweredOn
    public var services: [CBMutableService] = []
    public var notifications: [(Data, CBCentral)] = []
    public var responses: [CBATTError.Code] = []
    public var hasCapacity = true
    public var isAdvertising = false
    public init(delegate: CBPeripheralManagerDelegate?, queue: DispatchQueue?) {
        self.delegate = delegate
        super.init()
        Self.latest = self
    }
    public func add(_ service: CBMutableService) { services.append(service) }
    public func removeAllServices() { services.removeAll() }
    public func stopAdvertising() { isAdvertising = false }
    public func startAdvertising(_ data: [String: Any]) { isAdvertising = true }
    public func updateValue(_ data: Data, for characteristic: CBMutableCharacteristic, onSubscribedCentrals: [CBCentral]?) -> Bool {
        guard hasCapacity else { return false }
        for central in onSubscribedCentrals ?? [] {
            precondition(data.count <= central.maximumUpdateValueLength, "Oversized ATT notification")
            notifications.append((data, central))
        }
        return true
    }
    public func respond(to request: CBATTRequest, withResult result: CBATTError.Code) { responses.append(result) }
}
