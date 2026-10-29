import AppKit

// Probe is the headless check: the page loaded off screen with the bridge, the alerts socket open, everything heard
// printed as JSON lines; no window, no menu bar item, no notification is asked for or shown.
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
        var ready = false
        let web = WebWindow(server: server, persistent: false)
        web.onBridge = { message, raw in
            if case .ready = message { ready = true }
            say(["bridge": raw])
        }
        let alerts = AlertSocket(server: server, label: "remuda probe", remember: false)
        alerts.onAlert = { _, text in
            say(["alert": (try? JSONSerialization.jsonObject(with: Data(text.utf8))) ?? text])
        }
        alerts.onState = { state, detail in say(["socket": state, "detail": detail]) }
        say(["probe": server.base.absoluteString, "version": appVersion, "seconds": seconds])
        web.load()
        alerts.start()
        DispatchQueue.main.asyncAfter(deadline: .now() + .seconds(seconds)) {
            say(["done": ready ? "ready" : "the page never said ready"])
            exit(ready ? 0 : 1)
        }
        app.run()
        return 0
    }
}

// say prints one JSON line.
func say(_ object: [String: Any]) {
    guard let data = try? JSONSerialization.data(withJSONObject: object), let line = String(data: data, encoding: .utf8) else { return }
    print(line)
    fflush(stdout)
}
