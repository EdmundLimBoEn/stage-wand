package systems.edmundlim.stagewand.protocol

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class TrackpadGestureTest {
    private fun point(id: Long = 1, x: Float = 0f, pressed: Boolean = true) = TouchPoint(id, x, 0f, pressed)

    @Test fun dragPauseAndReleaseDoesNotClick() {
        val gesture = TrackpadGesture()
        gesture.update(listOf(point()), 0)
        gesture.update(listOf(point(x = 100f)), 50)
        gesture.update(listOf(point(x = 100f)), 100)
        assertNull(gesture.update(listOf(point(x = 100f, pressed = false)), 150))
    }

    @Test fun returnToOriginDoesNotTurnDragIntoTap() {
        val gesture = TrackpadGesture()
        gesture.update(listOf(point()), 0)
        gesture.update(listOf(point(x = 100f)), 50)
        gesture.update(listOf(point()), 100)
        assertNull(gesture.update(listOf(point(pressed = false)), 150))
    }

    @Test fun tapsAndSequentialTwoFingerRelease() {
        val gesture = TrackpadGesture()
        gesture.update(listOf(point()), 0)
        assertEquals(Command.Click(Button.Left), gesture.update(listOf(point(pressed = false)), 100))
        gesture.update(listOf(point(), point(2)), 200)
        assertNull(gesture.update(listOf(point(pressed = false), point(2)), 250))
        assertEquals(2, gesture.pointerCount)
        assertEquals(Command.Click(Button.Right), gesture.update(listOf(point(2, pressed = false)), 300))
    }

    @Test fun scrollAndStaggeredReleaseDoesNotClick() {
        val gesture = TrackpadGesture()
        gesture.update(listOf(point(), point(2)), 0)
        gesture.update(listOf(point(x = 50f), point(2, x = 50f)), 50)
        gesture.update(listOf(point(x = 50f, pressed = false), point(2, x = 50f)), 100)
        assertNull(gesture.update(listOf(point(2, x = 50f, pressed = false)), 150))
    }

    @Test fun slowThreeFingerSwipeFiresOnce() {
        val gesture = TrackpadGesture()
        gesture.update((1L..3).map { point(it) }, 0)
        var commands = 0
        for (x in 1..100) {
            val command = gesture.update((1L..3).map { point(it, x.toFloat()) }, x.toLong())
            if (command != null) {
                assertEquals(Command.Chord(ChordName.SpaceRight), command)
                commands++
            }
        }
        assertEquals(1, commands)
        assertNull(gesture.update((1L..3).map { point(it, 100f, false) }, 200))
    }

    @Test fun longPressDoesNotClick() {
        val gesture = TrackpadGesture()
        gesture.update(listOf(point()), 0)
        assertNull(gesture.update(listOf(point(pressed = false)), 1000))
    }
}
