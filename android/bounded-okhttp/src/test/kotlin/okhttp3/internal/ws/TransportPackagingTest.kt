package okhttp3.internal.ws

import java.util.Collections
import org.junit.Assert.assertEquals
import org.junit.Test

class TransportPackagingTest {
    @Test fun replacedClassesHaveExactlyOneRuntimeDefinition() {
        for (name in listOf(
            "WebSocketReader", "WebSocketReader\$FrameCallback",
            "MessageInflater", "MessageInflaterKt"
        )) {
            val resource = "okhttp3/internal/ws/$name.class"
            val definitions = Collections.list(javaClass.classLoader.getResources(resource))
            assertEquals("$resource: $definitions", 1, definitions.size)
        }
    }
}
