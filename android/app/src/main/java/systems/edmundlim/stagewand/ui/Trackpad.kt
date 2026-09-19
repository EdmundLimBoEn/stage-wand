package systems.edmundlim.stagewand.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.input.pointer.PointerEventPass
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.unit.dp
import systems.edmundlim.stagewand.protocol.Button
import systems.edmundlim.stagewand.protocol.ChordName
import systems.edmundlim.stagewand.protocol.Command

@Composable
fun Trackpad(
    armed: Boolean,
    sensitivity: Float,
    onCommand: (Command) -> Unit,
    modifier: Modifier = Modifier
) {
    var moveAcc by remember { mutableStateOf(Offset.Zero) }
    var scrollAcc by remember { mutableStateOf(Offset.Zero) }
    var dragging by remember { mutableStateOf(false) }

    LaunchedEffect(armed, dragging) {
        if (!armed) {
            moveAcc = Offset.Zero
            scrollAcc = Offset.Zero
            return@LaunchedEffect
        }
        while (dragging) {
            withFrameNanos { }
            val move = moveAcc
            val scroll = scrollAcc
            moveAcc = Offset.Zero
            scrollAcc = Offset.Zero
            flush(move, scroll, onCommand)
        }
        flush(moveAcc, scrollAcc, onCommand)
        moveAcc = Offset.Zero
        scrollAcc = Offset.Zero
    }

    Box(
        modifier
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.4f), RoundedCornerShape(24.dp))
            .pointerInput(armed, sensitivity) {
                awaitPointerEventScope {
                    while (true) {
                        val event = awaitPointerEvent(PointerEventPass.Main)
                        if (!armed) continue
                        val pressed = event.changes.filter { it.pressed }
                        when (pressed.size) {
                            0 -> {
                                val ended = event.changes
                                dragging = false
                                val slop = 18f
                                val travel = ended.fold(0f) { acc, change ->
                                    acc + (change.position - change.previousPosition).getDistance()
                                }
                                if (ended.size == 1 && travel < slop) onCommand(Command.Click(Button.Left))
                                if (ended.size >= 2 && travel < slop) onCommand(Command.Click(Button.Right))
                            }
                            1 -> {
                                dragging = true
                                val change = pressed[0]
                                val delta = change.position - change.previousPosition
                                val scale = 2f * sensitivity
                                moveAcc += delta * scale
                                change.consume()
                            }
                            2 -> {
                                dragging = true
                                val delta = pressed.fold(Offset.Zero) { acc, change ->
                                    acc + (change.position - change.previousPosition)
                                } / 2f
                                val scale = 2f * sensitivity
                                scrollAcc += delta * scale
                                pressed.forEach { it.consume() }
                            }
                            else -> {
                                dragging = false
                                val delta = pressed.fold(Offset.Zero) { acc, change ->
                                    acc + (change.position - change.previousPosition)
                                }
                                val chord = when {
                                    delta.x < -40 -> ChordName.SpaceLeft
                                    delta.x > 40 -> ChordName.SpaceRight
                                    delta.y < -40 -> ChordName.MissionControl
                                    else -> null
                                }
                                if (chord != null) onCommand(Command.Chord(chord))
                                pressed.forEach { it.consume() }
                            }
                        }
                    }
                }
            },
        contentAlignment = Alignment.TopCenter
    ) {
        Text(
            if (armed) "Drag to point · Tap to click" else "Arm to use the trackpad",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(12.dp)
        )
    }
}

private fun flush(move: Offset, scroll: Offset, onCommand: (Command) -> Unit) {
    var remaining = move
    while (remaining != Offset.Zero) {
        val dx = remaining.x.coerceIn(-400f, 400f)
        val dy = remaining.y.coerceIn(-400f, 400f)
        if (dx == 0f && dy == 0f) break
        onCommand(Command.Move(dx.toDouble(), dy.toDouble()))
        remaining = Offset(remaining.x - dx, remaining.y - dy)
    }
    if (scroll != Offset.Zero) {
        onCommand(Command.Scroll(scroll.x.toDouble(), scroll.y.toDouble()))
    }
}

private operator fun Offset.times(scale: Float) = Offset(x * scale, y * scale)
private operator fun Offset.div(value: Float) = Offset(x / value, y / value)
