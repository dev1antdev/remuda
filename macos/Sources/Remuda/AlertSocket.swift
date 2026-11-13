import Foundation

// AlertSocket is the app as a device of remuda's push service: registered with /api/push/socket, then every alert
// the phone would get arrives over /ws/alerts, and — asked for with withdraw=1 — the tags of the ones that no longer
// stand, a question answered elsewhere or a finish looked at, so they come down; the socket comes back by itself with
// a growing pause. Every callback of the session lands on the main queue, so the task, the backoff and the stop flag
// are only ever touched there.
final class AlertSocket {
    private static let endpointKey = "alerts.endpoint"
    private let server: Server
    private let label: String
    private let remember: Bool
    private let session = URLSession(configuration: .default, delegate: nil, delegateQueue: .main)
    private var task: URLSessionWebSocketTask?
    private var backoff: TimeInterval = 1
    private var stopped = false
    private var minted: String?
    private var prefs: [String: Any]? // the page's alert switches and quiet hours, which the server sends this device by
    var onAlert: ((Alert, String) -> Void)?
    var onWithdraw: (([String]) -> Void)?
    var onState: ((String, String) -> Void)?

    // init: the app remembers its device between launches so its prefs stay; a probe keeps its own for the run only.
    init(server: Server, label: String, remember: Bool = true) {
        self.server = server
        self.label = label
        self.remember = remember
    }

    // endpoint is the device's name on the server, kept per server so going back to one finds its device and prefs again;
    // the single key of before is the fallback until the server's own is written (empty once a 404 cleared it).
    var endpoint: String? {
        get {
            guard remember else { return minted }
            if let own = UserDefaults.standard.string(forKey: serverEndpointKey) { return own.isEmpty ? nil : own }
            return UserDefaults.standard.string(forKey: AlertSocket.endpointKey)
        }
        set {
            if remember {
                UserDefaults.standard.set(newValue ?? "", forKey: serverEndpointKey)
            } else {
                minted = newValue
            }
        }
    }

    private var serverEndpointKey: String { AlertSocket.endpointKey + "." + server.base.absoluteString }

    func start() {
        stopped = false
        open()
    }

    func stop() {
        stopped = true
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
    }

    // use takes the page's alert switches and quiet hours and hands them to the server for this device.
    func use(prefs: [String: Any]) {
        self.prefs = prefs
        guard !stopped else { return }
        Task { @MainActor in await self.register() }
    }

    private func open() {
        Task { @MainActor in
            await self.register()
            self.connect()
        }
    }

    // register mints the device, or renews its label and prefs when it exists already, one call at a time: prefs that
    // arrive while a call is out go in one more, so neither a second device is minted nor the page's prefs lost.
    private func register() async {
        if registering {
            registerAgain = true
            return
        }
        registering = true
        repeat {
            registerAgain = false
            await registerOnce()
        } while registerAgain && !stopped
        registering = false
    }

    private var registering = false
    private var registerAgain = false

    // registerOnce is one call; a device the server has forgotten is minted anew, with the prefs the page gave.
    private func registerOnce() async {
        var body: [String: Any] = ["label": label]
        if let endpoint { body["endpoint"] = endpoint }
        if let prefs { body["prefs"] = prefs }
        do {
            let (data, status) = try await server.request("/api/push/socket", method: "POST", json: body)
            if status == 404, endpoint != nil {
                endpoint = nil
                return await registerOnce()
            }
            guard status == 200,
                  let answer = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                  let named = answer["endpoint"] as? String else {
                report("error", "register answered \(status)")
                return
            }
            endpoint = named
            report("registered", named)
        } catch {
            report("error", "register: \(error.localizedDescription)")
        }
    }

    private func connect() {
        guard !stopped else { return }
        guard let endpoint else { return retry() }
        var request = URLRequest(url: server.socketURL("/ws/alerts", query: [URLQueryItem(name: "endpoint", value: endpoint), URLQueryItem(name: "withdraw", value: "1")]))
        request.setValue("Bearer " + server.token, forHTTPHeaderField: "Authorization")
        let socket = session.webSocketTask(with: request)
        task = socket
        socket.resume()
        socket.sendPing { [weak self] error in
            guard let self, error == nil, self.task === socket else { return }
            self.backoff = 1
            self.report("open", endpoint)
        }
        receive(on: socket)
    }

    private func receive(on socket: URLSessionWebSocketTask) {
        socket.receive { [weak self] result in
            guard let self, self.task === socket else { return }
            switch result {
            case .success(let message):
                switch message {
                case .string(let text): self.heard(text)
                case .data(let data): self.heard(String(decoding: data, as: UTF8.self))
                @unknown default: break
                }
                self.receive(on: socket)
            case .failure(let error):
                self.task = nil
                self.report("closed", error.localizedDescription)
                self.retry()
            }
        }
    }

    // heard is one frame: the tags of alerts taken back, or an alert.
    private func heard(_ text: String) {
        let data = Data(text.utf8)
        if let frame = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any], frame["type"] as? String == "withdraw" {
            let tags = frame["tags"] as? [String] ?? []
            DispatchQueue.main.async { self.onWithdraw?(tags) }
            return
        }
        guard let alert = Alert(json: data) else { return }
        DispatchQueue.main.async { self.onAlert?(alert, text) }
    }

    private func retry() {
        guard !stopped else { return }
        let delay = backoff
        backoff = min(backoff * 2, 30)
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self, !self.stopped, self.task == nil else { return }
            self.open()
        }
    }

    private func report(_ state: String, _ detail: String) {
        DispatchQueue.main.async { self.onState?(state, detail) }
    }
}
