import AppKit
import UserNotifications

// Notifier turns alerts into native notifications with the phone's buttons, and answers a button through remuda's routes.
final class Notifier: NSObject, UNUserNotificationCenterDelegate {
    // available says whether the process has a bundle: the notification centre refuses a bare binary.
    static var available: Bool { Bundle.main.bundleIdentifier != nil }

    // keep is how many sets of buttons stay registered: a question's buttons wear its own choices, so each wording is one.
    private static let keep = 32
    private let center = UNUserNotificationCenter.current()
    private var categories: [UNNotificationCategory] = []
    var server: Server
    var onOpen: ((_ machine: String, _ pane: String, _ panel: String) -> Void)?

    // init takes over the button sets registered before, so a notification still in the list keeps its buttons.
    init(server: Server) {
        self.server = server
        super.init()
        center.delegate = self
        center.getNotificationCategories { existing in
            DispatchQueue.main.async {
                let ours = Set(self.categories.map(\.identifier))
                self.categories = existing.filter { !ours.contains($0.identifier) } + self.categories
                self.register()
            }
        }
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

    // post shows one alert; its buttons are a category registered once per distinct set of actions and their titles.
    func post(_ alert: Alert) {
        let category = alert.actions.isEmpty ? "plain" : "acts:" + alert.actions.map { "\($0.action)=\($0.title)" }.joined(separator: "+")
        ensure(category: category, actions: alert.actions)
        let content = UNMutableNotificationContent()
        content.title = alert.title
        content.body = alert.body
        content.threadIdentifier = alert.tag.isEmpty ? alert.kind : alert.tag
        content.categoryIdentifier = category
        content.sound = .default
        content.userInfo = ["machine": alert.machine, "pane": alert.pane, "panel": alert.panel, "choices": alert.choices, "dialog": alert.dialog, "kind": alert.kind, "offer": alert.offer]
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        center.add(request) { error in
            if let error { NSLog("remuda: notification: %@", error.localizedDescription) }
        }
    }

    // ensure registers the category before its notification goes out, from the app's own list so two alerts in a row
    // cannot drop each other's buttons; the oldest sets past `keep` let go.
    private func ensure(category id: String, actions: [(action: String, title: String)]) {
        guard !categories.contains(where: { $0.identifier == id }) else { return }
        let buttons = actions.map { UNNotificationAction(identifier: $0.action, title: $0.title, options: $0.action == "open" ? [.foreground] : []) }
        categories.append(UNNotificationCategory(identifier: id, actions: buttons, intentIdentifiers: [], options: []))
        register()
    }

    private func register() {
        if categories.count > Notifier.keep { categories.removeFirst(categories.count - Notifier.keep) }
        center.setNotificationCategories(Set(categories))
    }

    func userNotificationCenter(_ center: UNUserNotificationCenter, willPresent notification: UNNotification, withCompletionHandler handler: @escaping (UNNotificationPresentationOptions) -> Void) {
        handler([.banner, .list, .sound])
    }

    // didReceive answers a button: Esc and a choice go to the pane through remuda, an offer runs on the other vendor,
    // anything else opens the window on it.
    // The centre calls it on a queue of its own, so the window is only ever opened from the main queue.
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse, withCompletionHandler handler: @escaping () -> Void) {
        let info = response.notification.request.content.userInfo
        let machine = info["machine"] as? String ?? ""
        let pane = info["pane"] as? String ?? ""
        let panel = info["panel"] as? String ?? ""
        let choices = info["choices"] as? [String] ?? []
        let dialog = info["dialog"] as? String ?? ""
        let offer = info["offer"] as? String ?? ""
        let action = response.actionIdentifier
        let open: () -> Void = { [weak self] in DispatchQueue.main.async { self?.onOpen?(machine, pane, panel) } }
        if action == "esc" {
            send("/api/keys", ["machine": machine, "pane": pane, "keys": ["esc"]], else: open, then: handler)
        } else if action.hasPrefix("choice-"), let index = Int(action.dropFirst("choice-".count)), index < choices.count {
            send("/api/question/answer", ["machine": machine, "pane": pane, "index": index, "label": choices[index], "dialog": dialog], else: open, then: handler)
        } else if action == "offer-run", !offer.isEmpty, let id = offer.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) {
            send("/api/limits/offers/\(id)/run", [:], else: open, then: handler)
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
