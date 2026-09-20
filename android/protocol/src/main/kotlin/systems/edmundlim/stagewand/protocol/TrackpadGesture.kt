package systems.edmundlim.stagewand.protocol

import kotlin.math.hypot

data class TouchPoint(val id: Long, val x: Float, val y: Float, val pressed: Boolean)

class TrackpadGesture(private val slop: Float = 18f) {
    private val origins = mutableMapOf<Long, TouchPoint>()
    private var startedAt = 0L
    private var travel = 0f
    private var chordSent = false
    var pointerCount = 0
        private set

    fun update(points: List<TouchPoint>, timeMillis: Long): Command? {
        if (origins.isEmpty()) {
            if (points.none { it.pressed }) return null
            startedAt = timeMillis
        }
        points.forEach { point ->
            val origin = origins.getOrPut(point.id) { point }
            travel = maxOf(travel, hypot(point.x - origin.x, point.y - origin.y))
        }
        val pressed = points.filter { it.pressed }
        pointerCount = maxOf(pointerCount, pressed.size)
        if (pressed.isEmpty()) {
            val click = if (!chordSent && travel < slop && timeMillis - startedAt <= 300) {
                when (pointerCount) {
                    1 -> Command.Click(Button.Left)
                    2 -> Command.Click(Button.Right)
                    else -> null
                }
            } else null
            origins.clear()
            pointerCount = 0
            travel = 0f
            chordSent = false
            return click
        }
        if (pressed.size >= 3 && !chordSent) {
            val dx = pressed.map { it.x - origins.getValue(it.id).x }.average()
            val dy = pressed.map { it.y - origins.getValue(it.id).y }.average()
            val chord = when {
                dx < -40 -> ChordName.SpaceLeft
                dx > 40 -> ChordName.SpaceRight
                dy < -40 -> ChordName.MissionControl
                else -> null
            }
            if (chord != null) {
                chordSent = true
                return Command.Chord(chord)
            }
        }
        return null
    }
}
