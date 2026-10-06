package dev.mardeus.herdrui

import com.intellij.execution.configurations.PathEnvironmentVariableUtil
import com.intellij.openapi.Disposable
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.service
import com.intellij.openapi.diagnostic.thisLogger
import java.io.File
import java.net.HttpURLConnection
import java.net.URI
import java.net.URLEncoder
import java.util.concurrent.TimeUnit

/**
 * The herdr-ui process on this host (the IDE backend in Remote Development):
 * found on PATH or in the usual places, started if not already answering, and
 * its access token read the way the binary itself reads it.
 *
 * Environment overrides: HERDR_UI_BIN (binary path), HERDR_UI_PORT.
 */
@Service(Service.Level.APP)
class HerdrUiService : Disposable {
    private val log = thisLogger()
    private var started: Process? = null

    val port: Int = System.getenv("HERDR_UI_PORT")?.toIntOrNull() ?: 7317
    val baseUrl: String get() = "http://127.0.0.1:$port"

    /** The herdr-ui binary, or null when none can be found. */
    fun binary(): File? {
        System.getenv("HERDR_UI_BIN")?.let { File(it) }?.takeIf { it.canExecute() }?.let { return it }
        PathEnvironmentVariableUtil.findInPath("herdr-ui")?.let { return it }
        val home = System.getProperty("user.home")
        return listOf("$home/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "$home/go/bin")
            .map { File(it, "herdr-ui") }
            .firstOrNull { it.canExecute() }
    }

    /** `herdr-ui -print-token`: honours HERDR_UI_TOKEN and the state dir. */
    fun token(): String? {
        val bin = binary() ?: return null
        return try {
            val p = ProcessBuilder(bin.path, "-print-token").redirectErrorStream(true).start()
            val out = p.inputStream.bufferedReader().readText().trim()
            if (p.waitFor(10, TimeUnit.SECONDS) && p.exitValue() == 0 && out.isNotEmpty()) out.lines().last() else null
        } catch (e: Exception) {
            log.warn("herdr-ui -print-token failed", e)
            null
        }
    }

    fun isUp(): Boolean = try {
        val c = URI("$baseUrl/api/health").toURL().openConnection() as HttpURLConnection
        c.connectTimeout = 1000
        c.readTimeout = 1000
        c.responseCode == 200
    } catch (_: Exception) {
        false
    }

    /** Starts herdr-ui if nothing answers on the port; true once it does. */
    @Synchronized
    fun ensureRunning(): Boolean {
        if (isUp()) return true
        val bin = binary() ?: return false
        if (started?.isAlive != true) {
            log.info("starting ${bin.path} on 127.0.0.1:$port")
            started = ProcessBuilder(bin.path, "-addr", "127.0.0.1:$port")
                .redirectErrorStream(true)
                .redirectOutput(File(System.getProperty("java.io.tmpdir"), "herdr-ui-plugin.log"))
                .start()
        }
        repeat(25) {
            if (isUp()) return true
            Thread.sleep(200)
        }
        return false
    }

    /** The URL that signs this browser in once and opens the project's pane. */
    fun loginUrl(projectDir: String?): String {
        val q = mutableListOf<String>()
        token()?.let { q += "token=$it" }
        projectDir?.let { q += "cwd=" + URLEncoder.encode(it, Charsets.UTF_8) }
        return if (q.isEmpty()) "$baseUrl/" else "$baseUrl/?" + q.joinToString("&")
    }

    override fun dispose() {
        started?.takeIf { it.isAlive }?.destroy()
    }

    companion object {
        fun get(): HerdrUiService = service()
    }
}
