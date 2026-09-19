package systems.edmundlim.stagewand.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.unit.dp
import systems.edmundlim.stagewand.protocol.SquareUnlock
import kotlin.math.hypot

@Composable
fun SquareUnlockGuide(onUnlock: () -> Unit) {
    var progress by remember { mutableStateOf(0.0) }
    var failed by remember { mutableStateOf(false) }
    var retry by remember { mutableStateOf(false) }
    val recognizer = remember { SquareUnlock() }
    var last by remember { mutableStateOf(Offset.Zero) }
    val mint = MaterialTheme.colorScheme.tertiary
    val error = MaterialTheme.colorScheme.error
    val outline = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.4f)

    Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
        Text("Touch controls locked", style = MaterialTheme.typography.titleLarge)
        Text("Use the volume buttons to change slides.", color = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.height(12.dp))
        Canvas(
            Modifier
                .size(260.dp)
                .pointerInput(Unit) {
                    fun norm(offset: Offset): Pair<Double, Double> {
                        val side = minOf(size.width, size.height).toFloat()
                        val origin = Offset((size.width - side) / 2f, (size.height - side) / 2f)
                        return ((offset.x - origin.x) / side).toDouble() to ((offset.y - origin.y) / side).toDouble()
                    }
                    detectDragGestures(
                        onDragStart = { offset ->
                            recognizer.reset()
                            retry = false
                            last = offset
                            val (x, y) = norm(offset)
                            recognizer.add(x, y, now())
                            progress = recognizer.progress
                            failed = recognizer.failed
                        },
                        onDrag = { change, _ ->
                            last = change.position
                            val (x, y) = norm(change.position)
                            recognizer.add(x, y, now())
                            progress = recognizer.progress
                            failed = recognizer.failed
                        },
                        onDragEnd = {
                            val (x, y) = norm(last)
                            val unlocked = recognizer.finish(x, y, now())
                            retry = !unlocked
                            progress = 0.0
                            failed = false
                            recognizer.reset()
                            if (unlocked) onUnlock()
                        }
                    )
                }
        ) {
            val side = size.minDimension - 48f
            val origin = Offset((size.width - side) / 2f, (size.height - side) / 2f)
            val path = Path().apply {
                moveTo(origin.x, origin.y)
                lineTo(origin.x + side, origin.y)
                lineTo(origin.x + side, origin.y + side)
                lineTo(origin.x, origin.y + side)
                close()
            }
            drawPath(
                path,
                outline,
                style = Stroke(width = 5f, pathEffect = PathEffect.dashPathEffect(floatArrayOf(8f, 6f)))
            )
            val trim = Path()
            var remaining = (progress / 4.0).toFloat().coerceIn(0f, 1f) * 4 * side
            val edges = listOf(
                origin to Offset(origin.x + side, origin.y),
                Offset(origin.x + side, origin.y) to Offset(origin.x + side, origin.y + side),
                Offset(origin.x + side, origin.y + side) to Offset(origin.x, origin.y + side),
                Offset(origin.x, origin.y + side) to origin
            )
            for ((start, end) in edges) {
                if (remaining <= 0f) break
                val span = end - start
                val len = hypot(span.x.toDouble(), span.y.toDouble()).toFloat()
                val take = remaining.coerceAtMost(len)
                val t = if (len == 0f) 0f else take / len
                if (trim.isEmpty) trim.moveTo(start.x, start.y)
                trim.lineTo(start.x + span.x * t, start.y + span.y * t)
                remaining -= take
            }
            drawPath(trim, if (failed) error else mint, style = Stroke(width = 7f, cap = StrokeCap.Round))
            listOf(
                origin,
                Offset(origin.x + side, origin.y),
                Offset(origin.x + side, origin.y + side),
                Offset(origin.x, origin.y + side)
            ).forEachIndexed { index, point ->
                drawCircle(mint.copy(alpha = if (index == 0) 1f else 0.5f), radius = 16f, center = point)
            }
        }
        Text(
            if (retry) "Try again slowly, keeping your finger on all four edges."
            else "Start at 1. Slowly trace the square clockwise in one continuous stroke.",
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.padding(horizontal = 16.dp)
        )
    }
}

private fun now(): Double = System.nanoTime() / 1_000_000_000.0
