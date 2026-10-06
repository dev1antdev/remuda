// JetBrains IDE plugin that embeds herdr-ui in a tool window. Built against
// IntelliJ IDEA Community (platform APIs only), so it installs into PhpStorm,
// GoLand, IDEA… including a Remote Development backend.
plugins {
    id("java")
    id("org.jetbrains.kotlin.jvm") version "2.2.20"
    id("org.jetbrains.intellij.platform") version "2.9.0"
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
        intellijIdeaCommunity(providers.gradleProperty("platformVersion").get())
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
