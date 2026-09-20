package systems.edmundlim.stagewand

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.KeyEvent
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.core.content.ContextCompat
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.ConnectionURL
import systems.edmundlim.stagewand.protocol.KeyName
import systems.edmundlim.stagewand.ui.StageWandScreen
import systems.edmundlim.stagewand.volume.KeepAliveService

class MainActivity : ComponentActivity() {
    private lateinit var link: Link
    private var settings by mutableStateOf(Settings())
    private var touchLocked by mutableStateOf(true)
    private var armed by mutableStateOf(false)
    private var showingSettings by mutableStateOf(false)
    private var volumePresses by mutableIntStateOf(0)
    private var tick by mutableIntStateOf(0)

    private val permissionLauncher = registerForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions()
    ) {
        if (settings.transport == "bluetooth" && !hasBluetoothPermission()) {
            link.markBluetoothDenied()
        } else {
            link.start()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settings = loadSettings()
        val discovery = Discovery(this)
        val bluetooth = BluetoothClient(this)
        link = Link(discovery, bluetooth)
        link.onChange = { tick++ }
        link.update(settings)
        KeepAliveService.listener = { up -> sendVolume(if (up) KeyName.Right else KeyName.Left) }
        handleDeepLink(intent)
        requestPermissions()
        startForegroundKeepAlive()
        setContent {
            tick
            MaterialTheme(colorScheme = darkColorScheme(tertiary = Color(0xFF3EB489))) {
                Surface(Modifier.fillMaxSize()) {
                    StageWandScreen(
                        link = link,
                        settings = settings,
                        touchLocked = touchLocked,
                        armed = armed,
                        showingSettings = showingSettings,
                        volumePresses = volumePresses,
                        onUnlock = { touchLocked = false },
                        onLock = { lockForPocket() },
                        onArmed = { armed = it },
                        onSettings = { showingSettings = it },
                        onSettingsChange = {
                            settings = it
                            saveSettings(it)
                            link.update(it)
                        },
                        onCommand = ::sendCommand
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleDeepLink(intent)
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent?): Boolean {
        if (event?.repeatCount != 0) return super.onKeyDown(keyCode, event)
        when (keyCode) {
            KeyEvent.KEYCODE_VOLUME_UP -> {
                sendVolume(KeyName.Right)
                return true
            }
            KeyEvent.KEYCODE_VOLUME_DOWN -> {
                sendVolume(KeyName.Left)
                return true
            }
        }
        return super.onKeyDown(keyCode, event)
    }

    override fun onPause() {
        super.onPause()
        lockForPocket()
    }

    override fun onDestroy() {
        link.close()
        KeepAliveService.listener = null
        stopService(Intent(this, KeepAliveService::class.java))
        super.onDestroy()
    }

    private fun lockForPocket() {
        touchLocked = true
        armed = false
        showingSettings = false
    }

    private fun sendVolume(key: KeyName) {
        volumePresses += 1
        link.send(Command.Key(key))
        Haptics.tick(window.decorView)
    }

    private fun sendCommand(command: Command) {
        if (touchLocked) return
        when (command) {
            is Command.Move, is Command.Click, is Command.Scroll, is Command.Chord -> if (!armed) return
            else -> {}
        }
        link.send(command)
        when (command) {
            is Command.Key, is Command.Click, is Command.Chord -> Haptics.tick(window.decorView)
            is Command.Move, is Command.Scroll -> Haptics.motion(window.decorView)
            is Command.Auth -> {}
        }
    }

    private fun handleDeepLink(intent: Intent?) {
        val uri = intent?.data?.toString() ?: return
        val dest = ConnectionURL.pairingDestination(uri) ?: return
        settings = settings.copy(manualHost = dest)
        saveSettings(settings)
        link.update(settings)
    }

    private fun loadSettings(): Settings {
        val prefs = getSharedPreferences("stagewand", Context.MODE_PRIVATE)
        return Settings(
            pairingCode = prefs.getString("pairingCode", "") ?: "",
            manualHost = prefs.getString("manualHost", "") ?: "",
            sensitivity = prefs.getFloat("sensitivity", 1f),
            transport = prefs.getString("transport", "bluetooth") ?: "bluetooth"
        )
    }

    private fun saveSettings(value: Settings) {
        getSharedPreferences("stagewand", Context.MODE_PRIVATE).edit()
            .putString("pairingCode", value.pairingCode)
            .putString("manualHost", value.manualHost)
            .putFloat("sensitivity", value.sensitivity)
            .putString("transport", value.transport)
            .apply()
    }

    private fun startForegroundKeepAlive() {
        val intent = Intent(this, KeepAliveService::class.java)
        if (Build.VERSION.SDK_INT >= 26) startForegroundService(intent) else startService(intent)
    }

    private fun requestPermissions() {
        val needed = mutableListOf<String>()
        if (Build.VERSION.SDK_INT >= 31) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_SCAN) != PackageManager.PERMISSION_GRANTED) {
                needed += Manifest.permission.BLUETOOTH_SCAN
            }
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_CONNECT) != PackageManager.PERMISSION_GRANTED) {
                needed += Manifest.permission.BLUETOOTH_CONNECT
            }
        }
        if (Build.VERSION.SDK_INT >= 33) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.NEARBY_WIFI_DEVICES) != PackageManager.PERMISSION_GRANTED) {
                needed += Manifest.permission.NEARBY_WIFI_DEVICES
            }
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                needed += Manifest.permission.POST_NOTIFICATIONS
            }
        } else if (Build.VERSION.SDK_INT >= 23) {
            if (ContextCompat.checkSelfPermission(this, Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
                needed += Manifest.permission.ACCESS_FINE_LOCATION
            }
        }
        if (needed.isEmpty()) {
            if (settings.transport == "bluetooth" && !hasBluetoothPermission()) {
                link.markBluetoothDenied()
            } else {
                link.start()
            }
        } else {
            permissionLauncher.launch(needed.toTypedArray())
        }
    }

    private fun hasBluetoothPermission(): Boolean {
        if (Build.VERSION.SDK_INT < 31) return true
        return ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_SCAN) == PackageManager.PERMISSION_GRANTED &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED
    }
}
