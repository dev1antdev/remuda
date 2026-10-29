import AppKit
import UserNotifications

// Notifier turns alerts into native notifications with the phone's buttons, and answers a button through remuda's routes.
final class Notifier: NSObject, UNUserNotificationCenterDelegate {
    // available says whether the process has a bundle: the notification centre refuses a bare binary.
    static var available: Bool { Bundle.main.bundleIdentifier != nil }

    private let center = UNUserNotificationCenter.current()
    private var categories: Set<String> = []
    var server: Server
    var onOpen: ((_ machine: String, _ pane: String, _ panel: String) -> Void)?

    init(server: Server) {
        self.server = server
        super.init()
        center.delegate = self
    }

    // askPermission puts the system's question up when it has not been answered yet.
    func askPermission(_ done: ((Bool) -> Void)? = nil) {
        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, _ in
            DispatchQueue.main.async { done?(granted) }
        }
    }

    // status words the permission as Settings shows it.
    func status(_ done: @escaping (String) -> Void) {
        center.getNotificationSettings { settings in
            let words: String
            switch settings.authorizationStatus {
            case .authorized, .provisional: words = "allowed"
            case .denied: words = "denied in System Settings"
            case .notDetermined: words = "not asked yet"
            default: words = "limited"
            }
            DispatchQueue.main.async { done(words) }
        }
    }

    // post shows one alert; its buttons are a category registered once per distinct set of actions.
    func post(_ alert: Alert) {
        let category = alert.actions.isEmpty ? "plain" : "acts:" + alert.actions.map { $0.action }.joined(separator: "+")
        ensure(category: category, actions: alert.actions) {
            let content = UNMutableNotificationContent()
            content.title = alert.title
            content.body = alert.body
            content.threadIdentifier = alert.tag.isEmpty ? alert.kind : alert.tag
            content.categoryIdentifier = category
            content.sound = .default
            content.userInfo = ["machine": alert.machine, "pane": alert.pane, "panel": alert.panel, "choices": alert.choices, "kind": alert.kind]
            let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
            self.center.add(request) { error in
                if let error { NSLog("remuda: notification: %@", error.localizedDescription) }
            }
        }
    }

    private func ensure(category id: String, actions: [(action: String, title: String)], then: @escaping () -> Void) {
        if categories.contains(id) { return then() }
        center.getNotificationCategories { existing in
            let buttons = actions.map { UNNotificationAction(identifier: $0.action, title: $0.title, options: $0.action == "open" ? [.foreground] : []) }
            let category = UNNotificationCategory(identifier: id, actions: buttons, intentIdentifiers: [], options: [])
            var all = existing.filter { $0.identifier != id }
            all.insert(category)
            self.center.setNotificationCategories(all)
            DispatchQueue.main.async {
                self.categories.insert(id)
                then()
            }
        }
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler handler: @escaping (UNNotificationPresentationOptions) -> Void) {
        handler([.banner, .list, .sound])
    }

    // didReceive answers a button: Esc and a choice go to the pane through remuda, anything else opens the window on it.
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse, withCompletionHandler handler: @escaping () -> Void) {
        let info = response.notification.request.content.userInfo
        let machine = info["machine"] as? String ?? ""
        let pane = info["pane"] as? String ?? ""
        let panel = info["panel"] as? String ?? ""
        let choices = info["choices"] as? [String] ?? []
        let action = response.actionIdentifier
        let open: () -> Void = { [weak self] in self?.onOpen?(machine, pane, panel) }
        if action == "esc" {
            send("/api/keys", ["machine": machine, "pane": pane, "keys": ["esc"]], else: open, then: handler)
        } else if action.hasPrefix("choice-"), let index = Int(action.dropFirst("choice-".count)), index < choices.count {
            send("/api/question/answer", ["machine": machine, "pane": pane, "index": index, "label": choices[index]], else: open, then: handler)
        } else {
            open()
            handler()
        }
    }

    private func send(_ path: String, _ body: [String: Any], else fallback: @escaping () -> Void, then done: @escaping () -> Void) {
        Task {
            let status = (try? await server.request(path, method: "POST", json: body))?.1 ?? 0
            await MainActor.run {
                if status != 200 { fallback() }
                done()
            }
        }
    }
}
