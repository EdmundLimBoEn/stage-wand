package systems.edmundlim.stagewand.volume

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.media.AudioManager
import android.media.MediaPlayer
import android.os.Build
import android.os.IBinder
import systems.edmundlim.stagewand.R

class KeepAliveService : Service() {
    private var player: MediaPlayer? = null
    private var lastVolume = -1
    private var suppressUntil = 0L

    private val receiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action != VOLUME_CHANGED) return
            val stream = intent.getIntExtra("android.media.EXTRA_VOLUME_STREAM_TYPE", -1)
            if (stream != AudioManager.STREAM_MUSIC) return
            if (System.currentTimeMillis() < suppressUntil) return
            val audio = getSystemService(AUDIO_SERVICE) as AudioManager
            val current = audio.getStreamVolume(AudioManager.STREAM_MUSIC)
            val previous = lastVolume
            lastVolume = current
            if (previous < 0 || current == previous) return
            listener?.invoke(current > previous)
        }
    }

    override fun onCreate() {
        super.onCreate()
        createChannel()
        startForeground(1, notification())
        player = MediaPlayer.create(this, R.raw.silence)?.apply {
            isLooping = true
            setVolume(0.01f, 0.01f)
            start()
        }
        val audio = getSystemService(AUDIO_SERVICE) as AudioManager
        lastVolume = audio.getStreamVolume(AudioManager.STREAM_MUSIC)
        val filter = IntentFilter(VOLUME_CHANGED)
        if (Build.VERSION.SDK_INT >= 33) {
            registerReceiver(receiver, filter, RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("DEPRECATION")
            registerReceiver(receiver, filter)
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    override fun onDestroy() {
        super.onDestroy()
        try {
            unregisterReceiver(receiver)
        } catch (_: IllegalArgumentException) {
        }
        player?.release()
        player = null
    }

    override fun onBind(intent: Intent?): IBinder? = null

    fun suppressSelf(ms: Long = 300) {
        suppressUntil = System.currentTimeMillis() + ms
    }

    private fun createChannel() {
        if (Build.VERSION.SDK_INT < 26) return
        val channel = NotificationChannel(CHANNEL, "Stage Wand", NotificationManager.IMPORTANCE_LOW)
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    private fun notification(): Notification {
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            Notification.Builder(this, CHANNEL)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        return builder
            .setContentTitle("Stage Wand")
            .setContentText("Listening for volume presses")
            .setSmallIcon(android.R.drawable.ic_media_play)
            .setOngoing(true)
            .build()
    }

    companion object {
        const val CHANNEL = "stagewand"
        const val VOLUME_CHANGED = "android.media.VOLUME_CHANGED_ACTION"
        var listener: ((up: Boolean) -> Unit)? = null
    }
}
