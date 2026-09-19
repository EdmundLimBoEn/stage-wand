package systems.edmundlim.stagewand

data class Settings(
    val pairingCode: String = "",
    val manualHost: String = "",
    val sensitivity: Float = 1f,
    val transport: String = "bluetooth"
)
