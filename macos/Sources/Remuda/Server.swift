import Foundation
import Security

// Server is the remuda the app talks to: its address and the token its own calls carry as a bearer.
struct Server {
    let base: URL
    let token: String

    static let defaultBase = URL(string: "http://127.0.0.1:7317")!
    static let keychainService = "remuda"
    private static let baseKey = "server.base"

    // ServerError says why a server could not be made.
    enum ServerError: LocalizedError {
        case noTokenFile(String)
        case badLink

        var errorDescription: String? {
            switch self {
            case .noTokenFile(let path): return "no remuda on this Mac: \(path) is missing (install remuda, or paste a pairing link in Settings)"
            case .badLink: return "a pairing link looks like http://host:7317/?token=…"
            }
        }
    }

    // tokenFile is where this Mac's own remuda keeps its token.
    static var tokenFile: URL {
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        return support.appendingPathComponent("remuda/token")
    }

    // local is this Mac's own remuda, signed in with the token file it keeps.
    static func local() throws -> Server {
        guard let raw = try? String(contentsOf: tokenFile, encoding: .utf8) else {
            throw ServerError.noTokenFile(tokenFile.path)
        }
        let token = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if token.isEmpty { throw ServerError.noTokenFile(tokenFile.path) }
        return Server(base: defaultBase, token: token)
    }

    // load is the server chosen last: a remote one paired in Settings, else this Mac's own.
    static func load() throws -> Server {
        if let base = UserDefaults.standard.string(forKey: baseKey), let url = URL(string: base), let token = Keychain.read(account: base) {
            return Server(base: url, token: token)
        }
        return try local()
    }

    // pair reads a pairing link, keeps the address and its token, and answers the server.
    static func pair(link: String) throws -> Server {
        guard let parts = URLComponents(string: link.trimmingCharacters(in: .whitespacesAndNewlines)),
              let token = parts.queryItems?.first(where: { $0.name == "token" })?.value, !token.isEmpty,
              let host = parts.host, let scheme = parts.scheme else {
            throw ServerError.badLink
        }
        var baseParts = URLComponents()
        baseParts.scheme = scheme
        baseParts.host = host
        baseParts.port = parts.port
        let base = baseParts.string!
        Keychain.write(account: base, secret: token)
        UserDefaults.standard.set(base, forKey: baseKey)
        return Server(base: URL(string: base)!, token: token)
    }

    // forgetPairing goes back to this Mac's own remuda.
    static func forgetPairing() {
        if let base = UserDefaults.standard.string(forKey: baseKey) {
            Keychain.delete(account: base)
        }
        UserDefaults.standard.removeObject(forKey: baseKey)
    }

    // isLocal says whether the server is this Mac's own.
    var isLocal: Bool { base == Server.defaultBase }

    // url is a path on the server.
    func url(_ path: String) -> URL { URL(string: path, relativeTo: base)!.absoluteURL }

    // signInURL is the page with the pairing token, which the server turns into a device cookie and strips.
    var signInURL: URL {
        var parts = URLComponents(url: base, resolvingAgainstBaseURL: false)!
        parts.path = "/"
        parts.queryItems = [URLQueryItem(name: "token", value: token)]
        return parts.url!
    }

    // socketURL is a WebSocket route with the same host.
    func socketURL(_ path: String, query: [URLQueryItem]) -> URL {
        var parts = URLComponents(url: base, resolvingAgainstBaseURL: false)!
        parts.scheme = parts.scheme == "https" ? "wss" : "ws"
        parts.path = path
        parts.queryItems = query
        return parts.url!
    }

    // request calls a route with the bearer and answers the body and the status.
    func request(_ path: String, method: String = "GET", json: Any? = nil) async throws -> (Data, Int) {
        var req = URLRequest(url: url(path))
        req.httpMethod = method
        req.setValue("Bearer " + token, forHTTPHeaderField: "Authorization")
        if let json {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONSerialization.data(withJSONObject: json)
        }
        let (data, response) = try await URLSession.shared.data(for: req)
        return (data, (response as? HTTPURLResponse)?.statusCode ?? 0)
    }
}

// Keychain keeps a paired server's token under the service "remuda", one item per address.
enum Keychain {
    private static func query(_ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: Server.keychainService, kSecAttrAccount as String: account]
    }

    static func read(account: String) -> String? {
        var q = query(account)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    static func write(account: String, secret: String) {
        delete(account: account)
        var q = query(account)
        q[kSecValueData as String] = Data(secret.utf8)
        SecItemAdd(q as CFDictionary, nil)
    }

    static func delete(account: String) {
        SecItemDelete(query(account) as CFDictionary)
    }
}
