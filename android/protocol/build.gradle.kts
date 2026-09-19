plugins {
    id("org.jetbrains.kotlin.jvm")
    id("org.jetbrains.kotlin.plugin.serialization")
}
kotlin {
    jvmToolchain(21)
}
dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3")
    testImplementation(kotlin("test"))
}
tasks.test {
    useJUnitPlatform()
}
sourceSets {
    getByName("test").resources.srcDir(rootProject.projectDir.resolve("../Shared"))
}
