import AppKit

// AppDelegate wires the pieces: the window, the menu bar item, the Dock badge, the alerts, the hotkey and Settings.
final class AppDelegate: NSObject, NSApplicationDelegate {
    private static let hotkeyKey = "hotkey"
    private var server: Server?
    private var web: WebWindow?
    private let status = StatusItem()
    private var alerts: AlertSocket?
    private var notifier: Notifier?
    private var hotkey: Hotkey?
    private let settings = SettingsWindow()
    private var hotkeySpec = UserDefaults.standard.string(forKey: AppDelegate.hotkeyKey) ?? Hotkey.defaultSpec

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.mainMenu = MainMenu.build(settings: #selector(openSettings), target: self)
        status.onSelect = { [weak self] row in self?.open(machine: row.machine, pane: row.pane, panel: "") }
        status.onOpen = { [weak self] in self?.web?.show() }
        status.onMode = { [weak self] mode in self?.web?.show(); self?.web?.openMode(mode) }
        status.onPanel = { [weak self] panel in self?.web?.show(); self?.web?.openPanel(panel) }
        status.onSettings = { [weak self] in self?.openSettings() }
        settings.onApply = { [weak self] text, spec in self?.apply(serverText: text, hotkey: spec) }
        settings.onAskNotifications = { [weak self] in self?.notifier?.askPermission { _ in self?.refreshSettings() } }
        bindHotkey()
        do {
            try connect(Server.load())
        } catch {
            showSettings(message: error.localizedDescription)
        }
    }

    // connect points everything at a remuda: the page signed in, the alerts socket as its device, the notifications.
    private func connect(_ next: Server) throws {
        server = next
        alerts?.stop()
        if let web {
            web.use(server: next)
        } else {
            let made = WebWindow(server: next)
            made.onBridge = { [weak self] message, _ in self?.bridge(message) }
            made.load()
            web = made
        }
        web?.show()
        if Notifier.available {
            let made = Notifier(server: next)
            made.onOpen = { [weak self] machine, pane, panel in self?.open(machine: machine, pane: pane, panel: panel) }
            made.askPermission()
            notifier = made
        }
        let socket = AlertSocket(server: next, label: "remuda on " + (Host.current().localizedName ?? "this Mac"))
        socket.onAlert = { [weak self] alert, _ in self?.notifier?.post(alert) }
        var lastTrouble = ""
        socket.onState = { state, detail in
            let trouble = state == "error" ? detail : ""
            if trouble != lastTrouble, !trouble.isEmpty { NSLog("remuda: alerts: %@", detail) }
            lastTrouble = trouble
        }
        alerts = socket
        socket.start()
    }

    private func bridge(_ message: Bridge) {
        switch message {
        case .badge(let count, let tone):
            status.setBadge(count: count, tone: tone)
            DockBadge.set(count: count)
        case .menu(let rows, let working, let idle):
            status.setMenu(rows: rows, working: working, idle: idle)
        case .ready, .other:
            break
        }
    }

    private func open(machine: String, pane: String, panel: String) {
        web?.show()
        if !panel.isEmpty {
            web?.openPanel(panel)
        } else if !pane.isEmpty {
            web?.select(machine: machine, pane: pane)
        }
    }

    private func bindHotkey() {
        hotkey = Hotkey(spec: hotkeySpec) { [weak self] in self?.web?.toggle() }
        status.hotkeyLabel = Hotkey.label(hotkeySpec)
    }

    @objc private func openSettings() { showSettings(message: "") }

    private func showSettings(message: String) {
        settings.show(server: server, hotkey: hotkeySpec, permission: "", message: message)
        refreshSettings()
    }

    private func refreshSettings() {
        guard let notifier else { return settings.say("notifications need the app bundle (make mac)") }
        notifier.status { [weak self] words in self?.settings.show(server: self?.server, hotkey: self?.hotkeySpec ?? "", permission: words, message: "") }
    }

    // apply takes the Settings form: a pairing link pairs, an empty field goes back to this Mac's own remuda.
    private func apply(serverText: String, hotkey spec: String) {
        if Hotkey.parse(spec) == nil {
            return settings.say("the hotkey needs a modifier and one key, like opt+cmd+r")
        }
        if spec != hotkeySpec {
            hotkeySpec = spec
            UserDefaults.standard.set(spec, forKey: AppDelegate.hotkeyKey)
            bindHotkey()
        }
        do {
            let text = serverText.trimmingCharacters(in: .whitespacesAndNewlines)
            let next: Server
            if text.isEmpty || text == "local" {
                Server.forgetPairing()
                next = try Server.local()
            } else if text.contains("token=") {
                next = try Server.pair(link: text)
            } else if let current = server, text == current.base.absoluteString {
                next = current
            } else {
                return settings.say("paste the pairing link of that remuda (its address with ?token=…)")
            }
            if next.base != server?.base || next.token != server?.token {
                try connect(next)
            }
            settings.say("applied")
        } catch {
            settings.say(error.localizedDescription)
        }
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        web?.show()
        return false
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}
