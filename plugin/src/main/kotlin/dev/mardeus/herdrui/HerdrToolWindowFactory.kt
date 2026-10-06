package dev.mardeus.herdrui

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.service
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory
import com.intellij.util.ui.JBUI
import com.intellij.ui.components.JBLabel
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

class HerdrToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val holder = JPanel(BorderLayout()).apply {
            add(JBLabel("Connecting to herdr-ui…").apply { border = JBUI.Borders.empty(16) }, BorderLayout.NORTH)
        }
        val content = ContentFactory.getInstance().createContent(holder, "", false)
        toolWindow.contentManager.addContent(content)

        // starting / probing the binary must not block the EDT
        ApplicationManager.getApplication().executeOnPooledThread {
            val svc = HerdrUiService.get()
            val view: HerdrView = when {
                svc.binary() == null -> FallbackView(
                    "<b>herdr-ui binary not found</b> on this host.<br><br>Install it next to herdr " +
                        "(<code>~/.local/bin/herdr-ui</code>, or set <code>HERDR_UI_BIN</code>) and reopen this window.",
                    null,
                )
                !svc.ensureRunning() -> FallbackView(
                    "<b>herdr-ui did not start</b> on 127.0.0.1:${svc.port}.<br><br>Is herdr running? " +
                        "See <code>\$TMPDIR/herdr-ui-plugin.log</code>.",
                    null,
                )
                EmbeddedView.supported() -> EmbeddedView(project, content, svc.loginUrl(project.basePath))
                else -> FallbackView(
                    "<b>No embedded browser here.</b><br><br>In Remote Development the backend's JCEF is off by default: " +
                        "set <code>REMOTE_DEV_SERVER_JCEF_ENABLED=true</code> in the host's login-shell environment " +
                        "(e.g. <code>~/.zshenv</code>) and restart the backend. Until then, open herdr-ui in a browser " +
                        "— Gateway forwards the port to this machine.",
                    svc.loginUrl(project.basePath),
                )
            }
            ApplicationManager.getApplication().invokeLater {
                HerdrProjectState.get(project).view = view
                holder.removeAll()
                holder.add(view.component, BorderLayout.CENTER)
                holder.revalidate()
                holder.repaint()
            }
        }
    }
}
