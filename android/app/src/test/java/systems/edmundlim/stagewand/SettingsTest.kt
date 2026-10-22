package systems.edmundlim.stagewand

import org.junit.Assert.*
import org.junit.Test
import java.net.URLEncoder

class SettingsTest {
    private val saved = Settings(pairingCode = "1234", manualHost = "wss://trusted.example:443/", sensitivity = 2f)

    @Test
    fun changedDestinationClearsCodeAndPreservesOtherSettings() {
        for (destination in listOf(
            "wss://attacker.example:443/",
            "wss://trusted.example:444/",
            "ws://trusted.example:443/",
            "wss://trusted.example:443/other",
            "wss://trusted.example:443/?route=other"
        )) {
            val updated = saved.withDeepLink(deepLink(destination))
            assertEquals(saved.copy(manualHost = destination, pairingCode = ""), updated)
        }
    }

    @Test
    fun movingFromDiscoveryToManualDestinationClearsCode() {
        val updated = saved.copy(manualHost = "").withDeepLink(deepLink(saved.manualHost))
        assertEquals("", updated.pairingCode)
        assertEquals(saved.manualHost, updated.manualHost)
    }

    @Test
    fun sameParsedDestinationRetainsCode() {
        for (host in listOf(saved.manualHost, " https://trusted.example ")) {
            assertEquals(saved, saved.copy(manualHost = host).withDeepLink(deepLink("https://trusted.example")))
        }
    }

    @Test
    fun invalidDeepLinkLeavesSettingsUnchanged() {
        for (uri in listOf("stagewand://connect?url=javascript:bad", "https://attacker.example", "stagewand://connect")) {
            assertEquals(saved, saved.withDeepLink(uri))
        }
    }

    private fun deepLink(destination: String) =
        "stagewand://connect?url=" + URLEncoder.encode(destination, "UTF-8")
}
