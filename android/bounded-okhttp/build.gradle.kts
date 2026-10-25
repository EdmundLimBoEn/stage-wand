plugins {
    id("org.jetbrains.kotlin.jvm")
    `java-library`
}

kotlin { jvmToolchain(17) }

// Keep the upstream transport, replacing only its unbounded receive reader/inflater.
// Never put the original jar on the runtime classpath alongside the replacements.
val upstream by configurations.creating { isTransitive = false }
val boundedTransport by tasks.registering(Jar::class) {
    archiveBaseName.set("okhttp-4.12.0-receive-bounded")
    from(upstream.map { zipTree(it) })
    exclude("okhttp3/internal/ws/WebSocketReader*.class")
    // Include Kotlin's generated MessageInflaterKt file facade.
    exclude("okhttp3/internal/ws/MessageInflater*.class")
}

// The replacement reader uses upstream's internal protocol constants.
tasks.named<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>("compileKotlin") {
    compilerOptions.freeCompilerArgs.add(boundedTransport.flatMap { it.archiveFile }.map {
        "-Xfriend-paths=" + it.asFile.absolutePath
    })
}

dependencies {
    upstream("com.squareup.okhttp3:okhttp:4.12.0")
    api(files(boundedTransport))
    api("com.squareup.okio:okio:3.6.0")
    testImplementation("junit:junit:4.13.2")
}
