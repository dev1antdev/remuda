// JetBrains IDE plugin that embeds herdr-ui in a tool window. Built against
// IntelliJ IDEA 2026.2 (platform APIs + the bundled JCEF plugin), so
// it installs into PhpStorm, GoLand, IDEA… including a Remote Development backend.
plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.4.20"
    id("org.jetbrains.intellij.platform") version "2.19.0"
}

group = "dev.mardeus"
version = providers.gradleProperty("pluginVersion").get()

repositories {
    mavenCentral()
    intellijPlatform {
        defaultRepositories()
    }
}

dependencies {
    intellijPlatform {
        // 2025.3+: one IntelliJ IDEA artifact (the Community/Ultimate split is gone)
        intellijIdea(providers.gradleProperty("platformVersion").get())
        // since 2026.x the embedded browser is its own bundled plugin
        bundledPlugin("com.intellij.modules.jcef")
    }
}

kotlin {
    jvmToolchain(21)
}

intellijPlatform {
    pluginConfiguration {
        name = "herdr-ui"
        version = providers.gradleProperty("pluginVersion").get()
        ideaVersion {
            sinceBuild = providers.gradleProperty("pluginSinceBuild").get()
            untilBuild = provider { null } // no upper bound: platform APIs used here are stable
        }
    }
    buildSearchableOptions = false
}
