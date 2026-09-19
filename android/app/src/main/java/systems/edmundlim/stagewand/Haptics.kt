package systems.edmundlim.stagewand

import android.view.HapticFeedbackConstants
import android.view.View

object Haptics {
    private var lastMotion = 0L

    fun tick(view: View?) {
        view?.performHapticFeedback(HapticFeedbackConstants.KEYBOARD_TAP)
    }

    fun motion(view: View?) {
        val now = System.currentTimeMillis()
        if (now - lastMotion < 80) return
        lastMotion = now
        view?.performHapticFeedback(HapticFeedbackConstants.CLOCK_TICK)
    }
}
