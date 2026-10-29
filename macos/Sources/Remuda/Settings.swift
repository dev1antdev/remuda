import AppKit
import ServiceManagement

// SettingsWindow is the small window: which remuda, the hotkey, launch at login and the notification permission.
final class SettingsWindow: NSObject, NSWindowDelegate {
    let window: NSWindow
    private let serverField = NSTextField()
    private let hotkeyField = NSTextField()
    private let loginBox = NSButton(checkboxWithTitle: "Launch at login", target: nil, action: nil)
    private let permission = NSTextField(labelWithString: "")
    private let note = NSTextField(wrappingLabelWithString: "")
    var onApply: ((_ serverText: String, _ hotkey: String) -> Void)?
    var onAskNotifications: (() -> Void)?

    override init() {
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 480, height: 300), styleMask: [.titled, .closable], backing: .buffered, defer: false)
        super.init()
        window.title = "remuda Settings"
        window.isReleasedWhenClosed = false
        window.delegate = self
        window.contentView = layout()
        loginBox.target = self
        loginBox.action = #selector(toggleLogin)
    }

    // show fills the fields from what is set now and brings the window up.
    func show(server: Server?, hotkey: String, permission words: String, message: String) {
        serverField.stringValue = server.map { $0.isLocal ? "" : $0.base.absoluteString } ?? ""
        hotkeyField.stringValue = hotkey
        permission.stringValue = "Notifications: " + words
        note.stringValue = message
        loginBox.state = SMAppService.mainApp.status == .enabled ? .on : .off
        window.center()
        window.makeKeyAndOrderFront(nil)
        activateApp()
    }

    func say(_ message: String) {
        note.stringValue = message
    }

    private func layout() -> NSView {
        serverField.placeholderString = "this Mac's remuda, or a pairing link http://host:7317/?token=…"
        hotkeyField.placeholderString = Hotkey.defaultSpec
        note.maximumNumberOfLines = 3
        note.textColor = .secondaryLabelColor
        let ask = NSButton(title: "Allow…", target: self, action: #selector(askNotifications))
        let permissionRow = NSStackView(views: [permission, ask])
        permissionRow.orientation = .horizontal
        let apply = NSButton(title: "Apply", target: self, action: #selector(apply))
        apply.keyEquivalent = "\r"
        let buttons = NSStackView(views: [NSView(), apply])
        buttons.orientation = .horizontal
        let stack = NSStackView(views: [
            labelled("Server", serverField),
            labelled("Hotkey", hotkeyField),
            loginBox,
            permissionRow,
            note,
            buttons,
        ])
        stack.orientation = .vertical
        stack.alignment = .leading
        stack.spacing = 12
        stack.edgeInsets = NSEdgeInsets(top: 20, left: 20, bottom: 20, right: 20)
        for view in [serverField, hotkeyField, note, buttons] as [NSView] {
            view.widthAnchor.constraint(equalToConstant: 440).isActive = true
        }
        return stack
    }

    private func labelled(_ title: String, _ field: NSTextField) -> NSView {
        let label = NSTextField(labelWithString: title)
        label.textColor = .secondaryLabelColor
        let row = NSStackView(views: [label, field])
        row.orientation = .vertical
        row.alignment = .leading
        row.spacing = 4
        return row
    }

    @objc private func apply() {
        onApply?(serverField.stringValue, hotkeyField.stringValue.isEmpty ? Hotkey.defaultSpec : hotkeyField.stringValue)
    }

    @objc private func askNotifications() { onAskNotifications?() }

    @objc private func toggleLogin() {
        do {
            if loginBox.state == .on {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
        } catch {
            note.stringValue = "launch at login: \(error.localizedDescription)"
            loginBox.state = SMAppService.mainApp.status == .enabled ? .on : .off
        }
    }
}
