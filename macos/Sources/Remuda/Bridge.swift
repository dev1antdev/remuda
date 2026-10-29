import Foundation

// Tone is how pressing an agent's need is: red for a question, amber for a watchdog note, green for a finish.
enum Tone: String {
    case red, amber, green
}

// MenuRow is one agent that needs the person, as the page lists it for the menu bar.
struct MenuRow {
    let machine: String
    let machineLabel: String
    let pane: String
    let workspace: String
    let title: String
    let said: String
    let tone: Tone?
    let state: String

    init?(_ d: [String: Any]) {
        guard let machine = d["machine"] as? String, let pane = d["pane"] as? String else { return nil }
        self.machine = machine
        self.pane = pane
        machineLabel = d["machineLabel"] as? String ?? machine
        workspace = d["workspace"] as? String ?? ""
        title = d["title"] as? String ?? pane
        said = d["said"] as? String ?? ""
        tone = Tone(rawValue: d["tone"] as? String ?? "")
        state = d["state"] as? String ?? ""
    }
}

// Bridge is a message the page posted through `webkit.messageHandlers.remuda`.
enum Bridge {
    case ready(version: String, machine: String)
    case badge(count: Int, tone: Tone?)
    case menu(rows: [MenuRow], working: Int, idle: Int)
    case other(type: String)

    init?(_ body: Any) {
        guard let d = body as? [String: Any], let type = d["type"] as? String else { return nil }
        switch type {
        case "ready":
            self = .ready(version: d["version"] as? String ?? "", machine: d["machine"] as? String ?? "")
        case "badge":
            self = .badge(count: d["count"] as? Int ?? 0, tone: Tone(rawValue: d["tone"] as? String ?? ""))
        case "menu":
            let rows = (d["rows"] as? [[String: Any]] ?? []).compactMap(MenuRow.init)
            self = .menu(rows: rows, working: d["working"] as? Int ?? 0, idle: d["idle"] as? Int ?? 0)
        default:
            self = .other(type: type)
        }
    }
}

// Alert is one frame off /ws/alerts: what a phone would have been pushed.
struct Alert {
    let machine: String
    let pane: String
    let title: String
    let body: String
    let tag: String
    let kind: String
    let actions: [(action: String, title: String)]
    let panel: String
    let choices: [String]

    init?(json: Data) {
        guard let d = (try? JSONSerialization.jsonObject(with: json)) as? [String: Any] else { return nil }
        machine = d["machine"] as? String ?? ""
        pane = d["pane"] as? String ?? ""
        title = d["title"] as? String ?? "remuda"
        body = d["body"] as? String ?? ""
        tag = d["tag"] as? String ?? ""
        kind = d["kind"] as? String ?? ""
        actions = (d["actions"] as? [[String: Any]] ?? []).compactMap { a in
            guard let action = a["action"] as? String, let title = a["title"] as? String else { return nil }
            return (action, title)
        }
        panel = d["panel"] as? String ?? ""
        choices = d["choices"] as? [String] ?? []
    }
}

// jsLiteral is a string as JavaScript source, for a call into the page.
func jsLiteral(_ s: String) -> String {
    guard let data = try? JSONSerialization.data(withJSONObject: s, options: .fragmentsAllowed), let text = String(data: data, encoding: .utf8) else {
        return "\"\""
    }
    return text
}

// appVersion is the bundle's version, "dev" outside a bundle.
let appVersion: String = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev"
