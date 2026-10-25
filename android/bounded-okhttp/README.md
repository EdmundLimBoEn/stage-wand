# Bounded OkHttp receive path

The app uses this module instead of the original OkHttp runtime jar. It retains
OkHttp 4.12.0 except for WebSocketReader and MessageInflater, copied from the
Apache-2.0 licensed upstream tag `parent-4.12.0`:

https://github.com/square/okhttp/tree/parent-4.12.0/okhttp/src/main/kotlin/okhttp3/internal/ws

Local changes enforce a cumulative 16 KiB wire payload limit before reading each
data fragment, and a 16 KiB inflation output limit before appending output.
Both text and binary messages are bounded. A one-byte overflow probe is separate
from the message buffer. Violations throw ProtocolException, which causes
RealWebSocket to fail and close the transport.

The build removes the original reader (including its callback interface) and
inflater classes before packaging their replacements, so there is one definition
of each at runtime. Do not add the original OkHttp dependency to the app.
On dependency upgrades, rebase these two files and run the reader and
RealWebSocket integration tests as well as the app's APK builds.
