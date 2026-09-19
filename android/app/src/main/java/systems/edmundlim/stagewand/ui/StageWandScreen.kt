package systems.edmundlim.stagewand.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Slider
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import systems.edmundlim.stagewand.Link
import systems.edmundlim.stagewand.Settings
import systems.edmundlim.stagewand.protocol.Button
import systems.edmundlim.stagewand.protocol.Command
import systems.edmundlim.stagewand.protocol.ConnectionURL
import systems.edmundlim.stagewand.protocol.KeyName

@Composable
fun StageWandScreen(
    link: Link,
    settings: Settings,
    touchLocked: Boolean,
    armed: Boolean,
    showingSettings: Boolean,
    volumePresses: Int,
    onUnlock: () -> Unit,
    onLock: () -> Unit,
    onArmed: (Boolean) -> Unit,
    onSettings: (Boolean) -> Unit,
    onSettingsChange: (Settings) -> Unit,
    onCommand: (Command) -> Unit
) {
    if (showingSettings) {
        SettingsPane(settings, onSettingsChange) { onSettings(false) }
        return
    }
    Column(
        Modifier
            .padding(16.dp)
            .verticalScroll(rememberScrollState()),
        verticalArrangement = Arrangement.spacedBy(12.dp)
    ) {
        Text("Stage Wand", style = MaterialTheme.typography.titleLarge)
        StatusPill(link)
        if (touchLocked) {
            SquareUnlockGuide(onUnlock)
        } else {
            Button(onClick = onLock, modifier = Modifier.fillMaxWidth()) { Text("Lock for pocket") }
            if (link.state == Link.State.EnterCode) {
                OutlinedTextField(
                    value = settings.pairingCode,
                    onValueChange = { onSettingsChange(settings.copy(pairingCode = it.filter(Char::isDigit).take(4))) },
                    label = { Text("Enter host code") },
                    modifier = Modifier.fillMaxWidth()
                )
            }
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                Text(if (armed) "ARMED" else "ARM POINTER", style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
                Switch(checked = armed, onCheckedChange = onArmed)
            }
            Trackpad(armed, settings.sensitivity, onCommand, Modifier.fillMaxWidth().height(220.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                CommandButton("←", Modifier.weight(1f)) { onCommand(Command.Key(KeyName.Left)) }
                CommandButton("→", Modifier.weight(1f)) { onCommand(Command.Key(KeyName.Right)) }
                CommandButton("ESC", Modifier.weight(1f)) { onCommand(Command.Key(KeyName.Esc)) }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                CommandButton("L CLICK", Modifier.weight(1f), enabled = armed) { onCommand(Command.Click(Button.Left)) }
                CommandButton("R CLICK", Modifier.weight(1f), enabled = armed) { onCommand(Command.Click(Button.Right)) }
            }
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                CommandButton("PREV", Modifier.weight(1f), large = true) { onCommand(Command.Key(KeyName.Left)) }
                CommandButton("NEXT", Modifier.weight(1f), large = true) { onCommand(Command.Key(KeyName.Right)) }
            }
            TextButton(onClick = { onSettings(true) }, modifier = Modifier.align(Alignment.End)) { Text("Settings") }
        }
        Text("Volume presses: $volumePresses", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun StatusPill(link: Link) {
    val text = when (link.state) {
        Link.State.Searching -> "Searching for your host…"
        Link.State.Connecting -> "Connecting to ${link.hostName ?: "host"}…"
        Link.State.EnterCode -> "Enter code · ${link.hostName ?: "host"}"
        Link.State.Authed -> "${link.route} · ${link.hostName ?: "host"}"
        Link.State.Disconnected -> "Disconnected · tap volume or reopen to reconnect"
        Link.State.LocalNetworkDenied -> "Local network denied. Allow nearby devices, or enter a host:port."
        Link.State.BluetoothDenied -> "Bluetooth denied. Allow Bluetooth, or switch to Wi-Fi nearby."
    }
    val color = if (link.state == Link.State.Authed) Color(0xFF3EB489) else Color(0xFFFFA000)
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant, CircleShape)
            .padding(horizontal = 14.dp, vertical = 10.dp)
    ) {
        Spacer(
            Modifier
                .size(8.dp)
                .clip(CircleShape)
                .background(color)
        )
        Text(text, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(start = 8.dp))
    }
}

@Composable
private fun CommandButton(label: String, modifier: Modifier, enabled: Boolean = true, large: Boolean = false, onClick: () -> Unit) {
    OutlinedButton(onClick = onClick, enabled = enabled, modifier = modifier.height(if (large) 68.dp else 44.dp)) {
        Text(label)
    }
}

@Composable
private fun SettingsPane(settings: Settings, onChange: (Settings) -> Unit, onDone: () -> Unit) {
    Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("Settings", style = MaterialTheme.typography.titleLarge)
        Text("Sensitivity ${"%.1f".format(settings.sensitivity)}")
        Slider(
            value = settings.sensitivity,
            onValueChange = { onChange(settings.copy(sensitivity = it)) },
            valueRange = 0.5f..3f
        )
        OutlinedTextField(
            value = settings.pairingCode,
            onValueChange = { onChange(settings.copy(pairingCode = it.filter(Char::isDigit).take(4))) },
            label = { Text("Pairing code") },
            modifier = Modifier.fillMaxWidth()
        )
        Text("Connection", style = MaterialTheme.typography.titleSmall)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
            if (settings.transport == "bluetooth") {
                Button(onClick = { onChange(settings.copy(transport = "bluetooth")) }, modifier = Modifier.weight(1f)) {
                    Text("Bluetooth")
                }
                OutlinedButton(onClick = { onChange(settings.copy(transport = "wifi")) }, modifier = Modifier.weight(1f)) {
                    Text("Wi-Fi")
                }
            } else {
                OutlinedButton(onClick = { onChange(settings.copy(transport = "bluetooth")) }, modifier = Modifier.weight(1f)) {
                    Text("Bluetooth")
                }
                Button(onClick = { onChange(settings.copy(transport = "wifi")) }, modifier = Modifier.weight(1f)) {
                    Text("Wi-Fi")
                }
            }
        }
        Text(
            "Bluetooth direct needs no Wi-Fi. Wi-Fi nearby uses _stagewand._tcp on the LAN. A host address or tunnel URL overrides both.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        OutlinedTextField(
            value = settings.manualHost,
            onValueChange = { onChange(settings.copy(manualHost = it)) },
            label = { Text("Host address or tunnel URL") },
            modifier = Modifier.fillMaxWidth()
        )
        val host = settings.manualHost.trim()
        if (host.isNotEmpty() && ConnectionURL.parse(host) == null) {
            Text(
                "Enter a valid host address or ws, wss, http, or https URL without a username, password, or fragment.",
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall
            )
        }
        Text(
            "Leave the address empty to use Bluetooth direct or LAN discovery.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
        Button(onClick = onDone, modifier = Modifier.fillMaxWidth()) { Text("Done") }
    }
}
