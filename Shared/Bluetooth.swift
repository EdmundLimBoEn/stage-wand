import Foundation

enum BluetoothProtocol {
    // Each ATT value is one complete UTF-8 JSON frame, with no fragmentation.
    static let maxFrameBytes = 512
    static let minimumCommandBytes = 80
    static let service = "5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01"
    static let command = "5A3E0002-8B6C-4B1E-9F8D-2C7A1D4E6F01"
    static let reply = "5A3E0003-8B6C-4B1E-9F8D-2C7A1D4E6F01"
}
