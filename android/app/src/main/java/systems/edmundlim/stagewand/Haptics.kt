package systems.edmundlim.stagewand

import android.content.Context
import android.os.Build
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.view.HapticFeedbackConstants
import android.view.View

object Haptics {
    private var lastMotion = 0L

    /**
     * Volume-button click confirmation. Uses the Vibrator rather than View haptics so it
     * still fires with the screen off, when presses arrive through KeepAliveService.
     */
    fun click(context: Context) {
        val vibrator = if (Build.VERSION.SDK_INT >= 31) {
            context.getSystemService(VibratorManager::class.java)?.defaultVibrator
        } else {
            @Suppress("DEPRECATION")
            context.getSystemService(Vibrator::class.java)
        } ?: return
        if (!vibrator.hasVibrator()) return
        val effect = if (Build.VERSION.SDK_INT >= 29) {
            VibrationEffect.createPredefined(VibrationEffect.EFFECT_TICK)
        } else {
            VibrationEffect.createOneShot(20, 80)
        }
        vibrator.vibrate(effect)
    }

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
