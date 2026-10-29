import Foundation

// AlertSocket is the app as a device of remuda's push service: registered with /api/push/socket, then every alert
// the phone would get arrives over /ws/alerts; the socket comes back by itself with a growing pause.
final class AlertSocket {
    private static let endpointKey = "alerts.endpoint"
    private let server: Server
    private let label: String
    private let remember: Bool
    private let session = URLSession(configuration: .default)
    private var task: URLSessionWebSocketTask?
    private var backoff: TimeInterval = 1
    private var stopped = false
    private var minted: String?
    var onAlert: ((Alert, String) -> Void)?
    var onState: ((String, String) -> Void)?

    // init: the app remembers its device between launches so its prefs stay; a probe keeps its own for the run only.
    init(server: Server, label: String, remember: Bool = true) {
        self.server = server
        self.label = label
        self.remember = remember
    }

    // endpoint is the device's name on the server.
    var endpoint: String? {
        get { remember ? UserDefaults.standard.string(forKey: AlertSocket.endpointKey) : minted }
        set {
            if remember {
                UserDefaults.standard.set(newValue, forKey: AlertSocket.endpointKey)
            } else {
                minted = newValue
            }
        }
    }

    func start() {
        stopped = false
        open()
    }

    func stop() {
        stopped = true
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
    }

    private func open() {
        Task { @MainActor in
            await self.register()
            self.connect()
        }
    }

    // register mints the device, or renews its label when it exists already; a device the server has forgotten is minted anew.
    private func register() async {
        var body: [String: Any] = ["label": label]
        if let endpoint { body["endpoint"] = endpoint }
        do {
            let (data, status) = try await server.request("/api/push/socket", method: "POST", json: body)
            if status == 404, endpoint != nil {
                endpoint = nil
                return await register()
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
        var request = URLRequest(url: server.socketURL("/ws/alerts", query: [URLQueryItem(name: "endpoint", value: endpoint)]))
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

    private func heard(_ text: String) {
        guard let alert = Alert(json: Data(text.utf8)) else { return }
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
