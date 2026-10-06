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

    private val home: String get() = System.getProperty("user.home")

    /** Where installers put binaries; an IDE backend's own PATH has none of these. */
    private val extraBinDirs: List<String>
        get() = listOf("$home/.local/bin", "/opt/homebrew/bin", "/usr/local/bin", "$home/go/bin")

    private fun find(name: String, override: String?): File? {
        override?.let { File(it) }?.takeIf { it.canExecute() }?.let { return it }
        PathEnvironmentVariableUtil.findInPath(name)?.let { return it }
        return extraBinDirs.map { File(it, name) }.firstOrNull { it.canExecute() }
    }

    /** The herdr-ui binary, or null when none can be found. */
    fun binary(): File? = find("herdr-ui", System.getenv("HERDR_UI_BIN"))

    /** The herdr binary herdr-ui needs for `terminal attach`. */
    fun herdrBinary(): File? = find("herdr", System.getenv("HERDR_BIN"))

    val logFile: File get() = File(System.getProperty("java.io.tmpdir"), "herdr-ui-plugin.log")

    /** The last lines herdr-ui logged — the reason when it did not start. */
    fun logTail(lines: Int = 5): String = try {
        logFile.readLines().takeLast(lines).joinToString("\n")
    } catch (_: Exception) {
        ""
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
            val args = mutableListOf(bin.path, "-addr", "127.0.0.1:$port")
            herdrBinary()?.let { args += listOf("-herdr-bin", it.path) }
            log.info("starting ${args.joinToString(" ")}")
            val pb = ProcessBuilder(args)
                .redirectErrorStream(true)
                .redirectOutput(logFile)
            // the backend's PATH is bare; herdr-ui and ssh/git it runs need the usual places
            pb.environment()["PATH"] = (extraBinDirs + (pb.environment()["PATH"] ?: "/usr/bin:/bin")).joinToString(":")
            started = pb.start()
        }
        for (i in 0 until 25) {
            if (isUp()) return true
            if (started?.isAlive == false) return false // died: the log says why
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
