package systems.edmundlim.stagewand.protocol

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlin.math.abs

private const val maxAbsMove = 400.0
private val json = Json { ignoreUnknownKeys = true }

sealed class Command {
    data class Auth(val code: String) : Command()
    data class Move(val dx: Double, val dy: Double) : Command()
    data class Click(val button: Button) : Command()
    data class Scroll(val dx: Double, val dy: Double) : Command()
    data class Key(val key: KeyName) : Command()
    data class Chord(val chord: ChordName) : Command()

    fun encode(): String = when (this) {
        is Auth -> obj("t" to "auth", "code" to code)
        is Move -> {
            require(dx.isFinite() && dy.isFinite()) { "nonfinite delta" }
            obj("t" to "move", "dx" to dx, "dy" to dy)
        }
        is Click -> obj("t" to "click", "b" to button.wire)
        is Scroll -> {
            require(dx.isFinite() && dy.isFinite()) { "nonfinite delta" }
            obj("t" to "scroll", "dx" to dx, "dy" to dy)
        }
        is Key -> obj("t" to "key", "k" to key.wire)
        is Chord -> obj("t" to "chord", "k" to chord.wire)
    }

    companion object {
        fun decode(text: String): Command? {
            val obj = parseObject(text) ?: return null
            return when (obj.string("t")) {
                "auth" -> obj.string("code")?.let(::Auth)
                "move", "scroll" -> {
                    val dx = obj.finite("dx") ?: return null
                    val dy = obj.finite("dy") ?: return null
                    if (obj.string("t") == "move") Move(dx, dy) else Scroll(dx, dy)
                }
                "click" -> Button.from(obj.string("b") ?: return null)?.let(::Click)
                "key" -> KeyName.from(obj.string("k") ?: return null)?.let(::Key)
                "chord" -> ChordName.from(obj.string("k") ?: return null)?.let(::Chord)
                else -> null
            }
        }
    }
}

enum class Button(val wire: String) {
    Left("left"), Right("right");
    companion object {
        fun from(value: String) = entries.find { it.wire == value }
    }
}

enum class KeyName(val wire: String) {
    Left("left"), Right("right"), Esc("esc");
    companion object {
        fun from(value: String) = entries.find { it.wire == value }
    }
}

enum class ChordName(val wire: String) {
    SpaceLeft("spaceLeft"), SpaceRight("spaceRight"), MissionControl("missionControl");
    companion object {
        fun from(value: String) = entries.find { it.wire == value }
    }
}

sealed class Reply {
    data object Status : Reply()
    data class Bye(val reason: String) : Reply()

    fun encode(): String = when (this) {
        is Status -> obj("t" to "status")
        is Bye -> obj("t" to "bye", "reason" to reason)
    }

    companion object {
        fun decode(text: String): Reply? {
            val obj = parseObject(text) ?: return null
            return when (obj.string("t")) {
                "status" -> Status
                "bye" -> obj.string("reason")?.let(::Bye)
                else -> null
            }
        }
    }
}

fun moveInRange(dx: Double, dy: Double): Boolean = abs(dx) <= maxAbsMove && abs(dy) <= maxAbsMove

private fun parseObject(text: String): JsonObject? =
    runCatching { json.parseToJsonElement(text).jsonObject }.getOrNull()

private fun JsonObject.string(key: String): String? =
    this[key]?.jsonPrimitive?.contentOrNull

private fun JsonObject.finite(key: String): Double? {
    val primitive = this[key]?.jsonPrimitive ?: return null
    val value = primitive.doubleOrNull ?: return null
    return value.takeIf { it.isFinite() }
}

private fun obj(vararg pairs: Pair<String, Any>): String = buildJsonObject {
    for ((key, value) in pairs) {
        when (value) {
            is String -> put(key, value)
            is Double -> put(key, value)
            is Number -> put(key, value.toDouble())
            else -> put(key, JsonPrimitive(value.toString()))
        }
    }
}.toString()
