import AppKit
import WebKit

// WebWindow is the one window: remuda's page in a WKWebView that keeps living while the window is hidden.
final class WebWindow: NSObject {
    let window: NSWindow
    let webView: WKWebView
    private(set) var server: Server
    var onBridge: ((Bridge, Any) -> Void)?

    // init makes the window; the cookie jar is the app's own and persists, or a throwaway one for a probe.
    init(server: Server, persistent: Bool = true) {
        self.server = server
        let config = WKWebViewConfiguration()
        config.websiteDataStore = persistent ? .default() : .nonPersistent()
        config.applicationNameForUserAgent = "remuda-mac/\(appVersion)"
        let announce = "window.remudaShell = Object.assign(window.remudaShell || {}, {native: true, version: \(jsLiteral(appVersion))});"
        config.userContentController.addUserScript(WKUserScript(source: announce, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        webView = WKWebView(frame: NSRect(x: 0, y: 0, width: 1280, height: 820), configuration: config)
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1280, height: 820),
                          styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        super.init()
        config.userContentController.add(WeakScriptHandler(self), name: "remuda")
        window.title = "remuda"
        window.isReleasedWhenClosed = false
        window.minSize = NSSize(width: 360, height: 400)
        window.contentView = webView
        window.delegate = self
        if !window.setFrameUsingName("remuda") {
            window.center()
        }
        window.setFrameAutosaveName("remuda")
        webView.navigationDelegate = self
        webView.uiDelegate = self
    }

    // use points the window at another remuda and loads it.
    func use(server: Server) {
        self.server = server
        load()
    }

    // load opens the page signed in: the pairing token goes once and the server strips it after setting the cookie.
    func load() {
        webView.load(URLRequest(url: server.signInURL))
    }

    func show() {
        window.makeKeyAndOrderFront(nil)
        activateApp()
    }

    func hide() {
        window.orderOut(nil)
    }

    // toggle hides the window when it is in front, else brings it up.
    func toggle() {
        if window.isVisible && NSApp.isActive && window.isKeyWindow {
            hide()
        } else {
            show()
        }
    }

    func select(machine: String, pane: String) { call("select", [machine, pane]) }
    func openPanel(_ name: String) { call("openPanel", [name]) }
    func openMode(_ mode: String) { call("openMode", [mode]) }

    private func call(_ function: String, _ args: [String]) {
        let list = args.map(jsLiteral).joined(separator: ", ")
        let source = "window.remudaShell && typeof window.remudaShell.\(function) === 'function' && window.remudaShell.\(function)(\(list)); undefined"
        webView.evaluateJavaScript(source) { _, _ in }
    }

    fileprivate func received(_ body: Any) {
        guard let message = Bridge(body) else { return }
        onBridge?(message, body)
    }

    private func isOurs(_ url: URL) -> Bool {
        url.host == server.base.host && (url.port ?? defaultPort(url.scheme)) == (server.base.port ?? defaultPort(server.base.scheme))
    }

    private func defaultPort(_ scheme: String?) -> Int { scheme == "https" ? 443 : 80 }
}

// WeakScriptHandler breaks the cycle WebKit would make by holding the window as its message handler.
private final class WeakScriptHandler: NSObject, WKScriptMessageHandler {
    weak var target: WebWindow?

    init(_ target: WebWindow) { self.target = target }

    func userContentController(_ controller: WKUserContentController, didReceive message: WKScriptMessage) {
        target?.received(message.body)
    }
}

extension WebWindow: NSWindowDelegate {
    // windowShouldClose hides the window instead: the app lives on in the menu bar.
    func windowShouldClose(_ sender: NSWindow) -> Bool {
        hide()
        return false
    }
}

extension WebWindow: WKNavigationDelegate, WKDownloadDelegate {
    func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = action.request.url else { return decisionHandler(.allow) }
        if action.shouldPerformDownload || url.scheme == "blob" {
            return decisionHandler(.download)
        }
        let external = (url.scheme == "http" || url.scheme == "https") && !isOurs(url)
        if external && action.targetFrame?.isMainFrame != false {
            NSWorkspace.shared.open(url)
            return decisionHandler(.cancel)
        }
        decisionHandler(.allow)
    }

    func webView(_ webView: WKWebView, decidePolicyFor response: WKNavigationResponse, decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void) {
        decisionHandler(response.canShowMIMEType ? .allow : .download)
    }

    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) {
        download.delegate = self
    }

    func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) {
        download.delegate = self
    }

    // decideDestinationUsing puts a file the page saves into Downloads under a name not taken yet.
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        let downloads = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first!
        completionHandler(freeName(in: downloads, for: suggestedFilename))
    }

    func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
        NSLog("remuda: download failed: %@", error.localizedDescription)
    }

    private func freeName(in folder: URL, for name: String) -> URL {
        let stem = (name as NSString).deletingPathExtension
        let ext = (name as NSString).pathExtension
        var candidate = folder.appendingPathComponent(name)
        var n = 2
        while FileManager.default.fileExists(atPath: candidate.path) {
            let numbered = ext.isEmpty ? "\(stem) \(n)" : "\(stem) \(n).\(ext)"
            candidate = folder.appendingPathComponent(numbered)
            n += 1
        }
        return candidate
    }
}

extension WebWindow: WKUIDelegate {
    // createWebViewWith handles window.open: the page's own address stays here, anything else goes to the browser.
    func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for action: WKNavigationAction, windowFeatures: WKWindowFeatures) -> WKWebView? {
        guard let url = action.request.url else { return nil }
        if isOurs(url) {
            webView.load(action.request)
        } else {
            NSWorkspace.shared.open(url)
        }
        return nil
    }

    func webView(_ webView: WKWebView, runJavaScriptAlertPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping () -> Void) {
        let alert = NSAlert()
        alert.messageText = message
        alert.runModal()
        completionHandler()
    }

    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert()
        alert.messageText = message
        alert.addButton(withTitle: "OK")
        alert.addButton(withTitle: "Cancel")
        completionHandler(alert.runModal() == .alertFirstButtonReturn)
    }
}

// activateApp brings the app in front, the way the running macOS allows.
func activateApp() {
    if #available(macOS 14, *) {
        NSApp.activate()
    } else {
        NSApp.activate(ignoringOtherApps: true)
    }
}
