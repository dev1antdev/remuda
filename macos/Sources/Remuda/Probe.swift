import AppKit

// Probe is the headless check: the page loaded off screen with the bridge, the alerts socket open, everything heard
// printed as JSON lines until all of `expected` has been heard or the seconds run out; no window, no menu bar item, no
// notification is asked for or shown, and the two devices it made on the server — the one the page signed in as, the
// one the socket minted — are removed on its way out.
struct Probe {
    var server: URL?
    var tokenFile: String?
    var seconds = 15

    // parse reads `--probe [--server URL] [--token-file PATH] [--seconds N]`, nil when the arguments are not a probe.
    static func parse(_ args: [String]) -> Probe? {
        guard args.contains("--probe") else { return nil }
        var probe = Probe()
        var i = 1
        while i < args.count {
            let next = i + 1 < args.count ? args[i + 1] : ""
            switch args[i] {
            case "--server": probe.server = URL(string: next); i += 1
            case "--token-file": probe.tokenFile = next; i += 1
            case "--seconds": probe.seconds = Int(next) ?? probe.seconds; i += 1
            default: break
            }
            i += 1
        }
        return probe
    }

    func run() -> Int32 {
        let app = NSApplication.shared
        app.setActivationPolicy(.prohibited)
        let token: String
        if let tokenFile {
            token = ((try? String(contentsOfFile: tokenFile, encoding: .utf8)) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        } else {
            token = (try? Server.local().token) ?? ""
        }
        guard !token.isEmpty else {
            say(["error": "no token: " + (tokenFile ?? Server.tokenFile.path)])
            return 1
        }
        let server = Server(base: self.server ?? Server.defaultBase, token: token)
        var heard: Set<String> = []
        var finished = false
        let web = WebWindow(server: server, persistent: false)
        let alerts = AlertSocket(server: server, label: "remuda probe", remember: false)
        let finish = {
            guard !finished else { return }
            finished = true
            let outcome = heard.contains("ready")
            say(["done": outcome ? "ready" : "the page never said ready", "heard": heard.sorted()])
            alerts.stop()
            let device = alerts.endpoint
            DispatchQueue.main.asyncAfter(deadline: .now() + .seconds(leaveWithin)) { exit(outcome ? 0 : 1) }
            web.signOut {
                Task {
                    if let device { _ = try? await server.request("/api/push/subscribe", method: "DELETE", json: ["endpoint": device]) }
                    exit(outcome ? 0 : 1)
                }
            }
        }
        let note = { (what: String) in
            heard.insert(what)
            if heard.isSuperset(of: Probe.expected) { finish() }
        }
        web.onBridge = { message, raw in
            guard !finished else { return }
            say(["bridge": raw])
            switch message {
            case .ready: note("ready")
            case .badge: note("badge")
            case .menu: note("menu")
            case .other: break
            }
        }
        alerts.onAlert = { _, text in
            guard !finished else { return }
            say(["alert": (try? JSONSerialization.jsonObject(with: Data(text.utf8))) ?? text])
        }
        alerts.onState = { state, detail in
            guard !finished else { return }
            say(["socket": state, "detail": detail])
            if state == "open" { note("open") }
        }
        say(["probe": server.base.absoluteString, "version": appVersion, "seconds": seconds])
        web.load()
        alerts.start()
        DispatchQueue.main.asyncAfter(deadline: .now() + .seconds(seconds)) { finish() }
        app.run()
        return 0
    }

    // leaveWithin is how long the probe gives the server to take its devices back before it exits regardless.
    private let leaveWithin = 5

    // expected is what a working app hears, after which the probe stops early: the page's ready, badge and menu, the socket open.
    static let expected: Set<String> = ["ready", "badge", "menu", "open"]
}

// say prints one JSON line.
func say(_ object: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: object), let line = String(data: data, encoding: .utf8) else { return }
    print(line)
    fflush(stdout)
}
