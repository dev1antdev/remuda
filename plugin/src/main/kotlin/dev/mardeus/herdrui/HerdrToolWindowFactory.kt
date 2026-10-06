package dev.mardeus.herdrui

import com.intellij.icons.AllIcons
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.service
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.components.JBLabel
import com.intellij.ui.content.ContentFactory
import com.intellij.util.ui.JBUI
import java.awt.BorderLayout
import javax.swing.JPanel

/** Per-project handle to whatever the tool window currently shows. */
@Service(Service.Level.PROJECT)
class HerdrProjectState {
    @Volatile var view: HerdrView? = null

    companion object {
        fun get(project: Project): HerdrProjectState = project.service()
    }
}

private fun String.escapeHtml() = replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

/** What the background probe decided; the view itself is built on the EDT. */
private sealed interface Plan {
    data class Embed(val url: String) : Plan
    data class Fallback(val message: String, val url: String?) : Plan
}

class HerdrToolWindowFactory : ToolWindowFactory, DumbAware {
    private val log = thisLogger()

    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val holder = JPanel(BorderLayout())
        val content = ContentFactory.getInstance().createContent(holder, "", false)
        toolWindow.contentManager.addContent(content)
        // Reconnect: after a herdr-ui update or a dead backend, without an IDE restart
        toolWindow.setTitleActions(listOf(object : AnAction("Reconnect", "Restart herdr-ui if needed and reload", AllIcons.Actions.Refresh) {
            override fun actionPerformed(e: AnActionEvent) = connect(project, holder, content)
        }))
        connect(project, holder, content)
    }

    private fun connect(project: Project, holder: JPanel, content: com.intellij.ui.content.Content) {
        val status = JBLabel("Connecting to herdr-ui…").apply { border = JBUI.Borders.empty(16) }
        ApplicationManager.getApplication().invokeLater {
            (HerdrProjectState.get(project).view as? EmbeddedView)?.dispose()
            HerdrProjectState.get(project).view = null
            holder.removeAll()
            holder.add(status, BorderLayout.NORTH)
            holder.revalidate()
            holder.repaint()
        }
        val say = { text: String -> ApplicationManager.getApplication().invokeLater { status.text = text } }

        // Probing and starting the binary happen off the EDT; Swing and the
        // embedded browser are only ever touched on it. Nothing here may
        // leave the window stuck on "Connecting…": every failure becomes a
        // view that says what happened.
        ApplicationManager.getApplication().executeOnPooledThread {
            val plan: Plan = try {
                val svc = HerdrUiService.get()
                when {
                    svc.binary() == null -> Plan.Fallback(
                        "<b>herdr-ui binary not found</b> on this host.<br><br>Install it next to herdr " +
                            "(<code>~/.local/bin/herdr-ui</code>, or set <code>HERDR_UI_BIN</code>) and reopen this window.",
                        null,
                    )
                    else -> {
                        say("Starting herdr-ui…")
                        if (!svc.ensureRunning()) Plan.Fallback(
                            "<b>herdr-ui did not start</b> on 127.0.0.1:${svc.port}.<br><br>Is herdr running on this host?<br><br>" +
                                "<code>${svc.logFile.path}</code>:<pre>${svc.logTail().escapeHtml()}</pre>",
                            null,
                        ) else {
                            say("Signing in…")
                            val url = svc.loginUrl(project.basePath)
                            if (EmbeddedView.supported()) Plan.Embed(url)
                            else Plan.Fallback(
                                "<b>No embedded browser here.</b><br><br>In Remote Development the backend's JCEF is off by default: " +
                                    "set <code>REMOTE_DEV_SERVER_JCEF_ENABLED=true</code> in the host's login-shell environment " +
                                    "(e.g. <code>~/.zshenv</code>) and restart the backend. Until then, open herdr-ui in a browser " +
                                    "— Gateway forwards the port to this machine.",
                                url,
                            )
                        }
                    }
                }
            } catch (e: Throwable) {
                log.warn("herdr-ui probe failed", e)
                Plan.Fallback("<b>herdr-ui could not be reached.</b><br><br><pre>${(e.message ?: e.toString()).escapeHtml()}</pre>", null)
            }

            ApplicationManager.getApplication().invokeLater {
                val view: HerdrView = when (plan) {
                    is Plan.Fallback -> FallbackView(plan.message, plan.url)
                    is Plan.Embed -> try {
                        say("Opening…")
                        EmbeddedView(project, content, plan.url)
                    } catch (e: Throwable) {
                        // JCEF said it was supported but could not be created (seen in
                        // Remote Development): the browser link still works
                        log.warn("embedded browser failed, falling back", e)
                        FallbackView(
                            "<b>The embedded browser could not start</b> (${(e.message ?: e.javaClass.simpleName).escapeHtml()}).<br><br>" +
                                "Open herdr-ui in a browser instead — Gateway forwards the port.",
                            plan.url,
                        )
                    }
                }
                HerdrProjectState.get(project).view = view
                holder.removeAll()
                holder.add(view.component, BorderLayout.CENTER)
                holder.revalidate()
                holder.repaint()
            }
        }
    }
}
