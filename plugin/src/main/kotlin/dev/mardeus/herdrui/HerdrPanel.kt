package dev.mardeus.herdrui

import com.intellij.ide.BrowserUtil
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.ui.components.JBLabel
import com.intellij.ui.jcef.JBCefApp
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import com.intellij.util.ui.JBUI
import org.cef.browser.CefBrowser
import org.cef.browser.CefFrame
import org.cef.handler.CefLoadHandlerAdapter
import java.awt.BorderLayout
import java.awt.FlowLayout
import java.awt.datatransfer.StringSelection
import java.awt.Toolkit
import javax.swing.JButton
import javax.swing.JComponent
import javax.swing.JPanel
import javax.swing.SwingUtilities

/** What a tool window shows: the embedded herdr-ui, or why it can't. */
interface HerdrView {
    val component: JComponent
    /** Push text into the chat composer (e.g. "@src/app.php"). */
    fun insert(text: String)
}

/**
 * herdr-ui in an embedded Chromium. The page gets `window.herdrHost.openFile`
 * so a file path the agent mentions opens in this IDE's editor — files on
 * other machines fall back to herdr-ui's own viewer.
 */
class EmbeddedView(private val project: Project, parent: Disposable, url: String) : HerdrView, Disposable {
    private val browser: JBCefBrowser = JBCefBrowser.createBuilder().setUrl(url).build()
    private val openQuery: JBCefJSQuery = JBCefJSQuery.create(browser as JBCefBrowserBase)
    // the page's copies go through the IDE's clipboard, which Remote
    // Development mirrors to the client — the browser's own would stay on the host
    private val copyQuery: JBCefJSQuery = JBCefJSQuery.create(browser as JBCefBrowserBase)

    override val component: JComponent get() = browser.component

    override fun dispose() {
        Disposer.dispose(openQuery)
        Disposer.dispose(copyQuery)
        Disposer.dispose(browser)
    }

    init {
        Disposer.register(parent, this)
        openQuery.addHandler { payload ->
            // payload: "<line>\t<path>"
            val (line, path) = payload.split('\t', limit = 2).let { it[0].toIntOrNull() to it.getOrElse(1) { "" } }
            if (path.isNotEmpty()) openInEditor(path, line)
            null
        }
        copyQuery.addHandler { text ->
            ApplicationManager.getApplication().invokeLater {
                CopyPasteManager.getInstance().setContents(StringSelection(text))
            }
            null
        }
        browser.jbCefClient.addLoadHandler(object : CefLoadHandlerAdapter() {
            override fun onLoadEnd(cefBrowser: CefBrowser, frame: CefFrame, httpStatusCode: Int) {
                if (!frame.isMain) return
                val openCall = openQuery.inject("payload")
                val copyCall = copyQuery.inject("text")
                cefBrowser.executeJavaScript(
                    """
                    window.herdrHost = {
                      openFile: function (path, line, machine) {
                        if (machine !== 'local') return false; // not this host's file
                        var payload = String(line || 0) + '\t' + path;
                        $openCall
                        return true;
                      },
                      copy: function (text) {
                        $copyCall
                        return true;
                      }
                    };
                    """.trimIndent(),
                    cefBrowser.url, 0,
                )
            }
        }, browser.cefBrowser)
    }

    private fun openInEditor(path: String, line: Int?) {
        ApplicationManager.getApplication().invokeLater {
            val vf = LocalFileSystem.getInstance().refreshAndFindFileByPath(path) ?: return@invokeLater
            OpenFileDescriptor(project, vf, (line ?: 1).coerceAtLeast(1) - 1, 0).navigate(true)
        }
    }

    override fun insert(text: String) {
        val json = "\"" + text.replace("\\", "\\\\").replace("\"", "\\\"") + "\""
        browser.cefBrowser.executeJavaScript("window.herdrUI && window.herdrUI.insert($json)", browser.cefBrowser.url, 0)
    }

    companion object {
        fun supported(): Boolean = JBCefApp.isSupported()
    }
}

/**
 * Shown when there is no embedded browser (Remote Development without
 * REMOTE_DEV_SERVER_JCEF_ENABLED, or no herdr-ui binary): the way out is a
 * real browser on the client, which Gateway reaches through port forwarding.
 */
class FallbackView(message: String, private val url: String?) : HerdrView {
    override val component: JComponent = JPanel(BorderLayout()).apply {
        border = JBUI.Borders.empty(16)
        add(JBLabel("<html><body style='width:280px'>$message</body></html>").apply { verticalAlignment = JBLabel.TOP }, BorderLayout.CENTER)
        if (url != null) {
            add(JPanel(FlowLayout(FlowLayout.LEFT, 8, 0)).apply {
                add(JButton("Open in browser").apply { addActionListener { BrowserUtil.browse(url) } })
                add(JButton("Copy link").apply {
                    addActionListener { Toolkit.getDefaultToolkit().systemClipboard.setContents(StringSelection(url), null) }
                })
            }, BorderLayout.SOUTH)
        }
    }

    override fun insert(text: String) {
        SwingUtilities.invokeLater { url?.let { BrowserUtil.browse(it) } }
    }
}
