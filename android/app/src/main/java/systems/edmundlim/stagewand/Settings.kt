package systems.edmundlim.stagewand

import systems.edmundlim.stagewand.protocol.ConnectionURL

data class Settings(
    val pairingCode: String = "",
    val manualHost: String = "",
    val sensitivity: Float = 1f,
    val transport: String = "bluetooth"
) {
    fun withDeepLink(uri: String): Settings {
        val destination = ConnectionURL.pairingDestination(uri) ?: return this
        // Clear credentials in the same update as the destination, before reconnect.
        return copy(
            manualHost = destination,
            pairingCode = if (ConnectionURL.parse(manualHost) == destination) pairingCode else ""
        )
    }
}
