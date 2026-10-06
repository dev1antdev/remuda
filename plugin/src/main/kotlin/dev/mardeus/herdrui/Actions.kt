package dev.mardeus.herdrui

import com.intellij.ide.BrowserUtil
import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.wm.ToolWindowManager
import java.nio.file.Path

/**
 * "Ask Agent About This File": mentions the file (project-relative, as
 * Claude Code's @path) in the chat composer — with the selected lines
 * quoted when the editor has a selection — and brings the tool window up.
 */
class AskAgentAction : AnAction() {
    override fun getActionUpdateThread() = ActionUpdateThread.BGT

    override fun update(e: AnActionEvent) {
        e.presentation.isEnabledAndVisible = e.project != null && e.getData(CommonDataKeys.VIRTUAL_FILE) != null
    }

    override fun actionPerformed(e: AnActionEvent) {
        val project = e.project ?: return
        val file = e.getData(CommonDataKeys.VIRTUAL_FILE) ?: return
        val base = project.basePath
        val rel = if (base != null && file.path.startsWith("$base/")) file.path.removePrefix("$base/") else file.path
        val editor = e.getData(CommonDataKeys.EDITOR)
        val sel = editor?.selectionModel
        val text = buildString {
            append('@').append(rel)
            if (sel != null && sel.hasSelection()) {
                val from = editor.document.getLineNumber(sel.selectionStart) + 1
                val to = editor.document.getLineNumber(sel.selectionEnd) + 1
                append(" (lines $from-$to):\n```\n").append(sel.selectedText).append("\n```\n")
            } else {
                append(' ')
            }
        }
        ToolWindowManager.getInstance(project).getToolWindow("herdr")?.activate {
            HerdrProjectState.get(project).view?.insert(text)
        }
    }
}

/** Opens herdr-ui for the project in a browser (through Gateway's port forwarding when remote). */
class OpenInBrowserAction : AnAction() {
    override fun getActionUpdateThread() = ActionUpdateThread.BGT

    override fun actionPerformed(e: AnActionEvent) {
        val svc = HerdrUiService.get()
        val dir = e.project?.basePath?.let { Path.of(it).toString() }
        if (svc.ensureRunning()) BrowserUtil.browse(svc.loginUrl(dir))
    }
}
