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
        case pairedUnusable(notice: String, local: Error)

        var errorDescription: String? {
            switch self {
            case .noTokenFile(let path): return "no remuda on this Mac: \(path) is missing (install remuda, or paste a pairing link in Settings)"
            case .badLink: return "a pairing link looks like http://host:7317/?token=…"
            case .pairedUnusable(let notice, let local): return notice + ", and " + local.localizedDescription
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

    // load is the server chosen last: a remote one paired in Settings, else this Mac's own. When the paired one's token
    // cannot be read, this Mac's own is used for this launch and the notice says so; the pairing stays for the next.
    static func load() throws -> (server: Server, notice: String?) {
        guard let base = UserDefaults.standard.string(forKey: baseKey), let url = URL(string: base) else { return (try local(), nil) }
        let trouble: String
        do {
            if let token = try Keychain.read(account: base) { return (Server(base: url, token: token), nil) }
            trouble = "its token is not in the login keychain"
        } catch {
            trouble = error.localizedDescription
        }
        let notice = "the remuda paired at \(base) could not be used: \(trouble)"
        do {
            return (try local(), notice + "; this Mac's own remuda is used instead (paste the pairing link again in Settings)")
        } catch {
            throw ServerError.pairedUnusable(notice: notice, local: error)
        }
    }

    // pair reads a pairing link, keeps the address and its token in place of the pairing before, and answers the server.
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
        try Keychain.write(account: base, secret: token)
        if let before = UserDefaults.standard.string(forKey: baseKey), before != base {
            Keychain.delete(account: before)
        }
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
    // KeychainError is a status the keychain answered, in the system's own words.
    struct KeychainError: LocalizedError {
        let doing: String
        let status: OSStatus

        var errorDescription: String? {
            let words = SecCopyErrorMessageString(status, nil) as String? ?? "status \(status)"
            return "the login keychain could not \(doing): \(words)"
        }
    }

    private static func query(_ account: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: Server.keychainService, kSecAttrAccount as String: account]
    }

    // read is the token kept for the address, nil when there is none; any other answer of the keychain is an error.
    static func read(account: String) throws -> String? {
        var q = query(account)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        let status = SecItemCopyMatching(q as CFDictionary, &out)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw KeychainError(doing: "read the token", status: status) }
        guard let data = out as? Data, let token = String(data: data, encoding: .utf8), !token.isEmpty else {
            throw KeychainError(doing: "read the token", status: errSecDecode)
        }
        return token
    }

    // write keeps the token for the address, replacing one kept before.
    static func write(account: String, secret: String) throws {
        let data = Data(secret.utf8)
        var status = SecItemUpdate(query(account) as CFDictionary, [kSecValueData as String: data] as CFDictionary)
        if status == errSecItemNotFound {
            var q = query(account)
            q[kSecValueData as String] = data
            status = SecItemAdd(q as CFDictionary, nil)
        }
        guard status == errSecSuccess else { throw KeychainError(doing: "keep the token", status: status) }
    }

    // delete removes the address's token; a failure is logged, since the pairing is let go either way.
    static func delete(account: String) {
        let status = SecItemDelete(query(account) as CFDictionary)
        if status != errSecSuccess && status != errSecItemNotFound {
            NSLog("remuda: %@", KeychainError(doing: "remove the token of \(account)", status: status).localizedDescription)
        }
    }
}
