package systems.edmundlim.stagewand.protocol

object BluetoothProtocol {
    // Each ATT value is one complete UTF-8 JSON frame, with no fragmentation.
    const val MAX_FRAME_BYTES = 512
    const val MIN_COMMAND_BYTES = 80
    const val SERVICE = "5A3E0001-8B6C-4B1E-9F8D-2C7A1D4E6F01"
    const val COMMAND = "5A3E0002-8B6C-4B1E-9F8D-2C7A1D4E6F01"
    const val REPLY = "5A3E0003-8B6C-4B1E-9F8D-2C7A1D4E6F01"
}
