// The native window owns its local scheme and shares one authenticated runtime with Capacitor.
import UIKit
import WebKit
import Capacitor
import OSLog

final class AgentWorkViewController: CAPBridgeViewController {
    private let agentWork = AgentWorkPlugin()
    private var configuration: InstanceConfiguration?
    private lazy var runtime: Result<RemoteRuntime, Error> = Result {
        guard let root = Bundle.main.url(forResource: "public", withExtension: nil),
              let service = Bundle.main.bundleIdentifier else { throw RemoteFailure.resource }
        do { return try RemoteRuntime(root: root, generated: AgentWorkRouter.generated, service: service + ".remote") }
        catch {
            Logger(subsystem: "local.agentwork.ios", category: "startup").error("Local resource initialization failed: \(String(describing: error), privacy: .public)")
            throw error
        }
    }

    override func router() -> Router { AgentWorkRouter() }

    override func webViewConfiguration(for configuration: InstanceConfiguration) -> WKWebViewConfiguration {
        self.configuration = configuration
        return super.webViewConfiguration(for: configuration)
    }

    override func webView(with frame: CGRect, configuration: WKWebViewConfiguration) -> WKWebView {
        // The SDK's original handler remains the fallback for its local file routes.
        // A new configuration admits our handler without replacing an already registered scheme.
        let owned = super.webViewConfiguration(for: self.configuration!)
        owned.userContentController = configuration.userContentController
        let fallback = configuration.urlSchemeHandler(forURLScheme: "dsh-app")!
        owned.setURLSchemeHandler(AgentWorkSchemeHandler(fallback: fallback, runtime: runtime), forURLScheme: "dsh-app")
        let file = Bundle.main.url(forResource: "carrier", withExtension: "js", subdirectory: "public/_ios")!
        let proof = "if(window===window.top&&location.protocol==='dsh-app:'&&location.hostname==='app'){Object.defineProperty(globalThis,'__AGENTWORK_NATIVE_NONCE__',{value:'\(agentWork.admission.nonce)'});}\n"
        let source = proof + (try! String(contentsOf: file, encoding: .utf8))
        owned.userContentController.addUserScript(WKUserScript(source: source, injectionTime: .atDocumentStart, forMainFrameOnly: false))
        return super.webView(with: frame, configuration: owned)
    }

    override func capacitorDidLoad() {
        agentWork.runtimeResult = runtime
        bridge?.registerPluginInstance(agentWork)
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        guard let web = webView else { return }
        let container = UIView()
        container.backgroundColor = .systemBackground
        container.addSubview(web)
        web.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            web.leadingAnchor.constraint(equalTo: container.safeAreaLayoutGuide.leadingAnchor),
            web.trailingAnchor.constraint(equalTo: container.safeAreaLayoutGuide.trailingAnchor),
            web.topAnchor.constraint(equalTo: container.safeAreaLayoutGuide.topAnchor),
            web.bottomAnchor.constraint(equalTo: container.keyboardLayoutGuide.topAnchor),
        ])
        view = container
    }

    override func viewDidAppear(_ animated: Bool) { super.viewDidAppear(animated); agentWork.resume() }
    func resumeConnections() { agentWork.resume() }
    deinit { agentWork.shutdown() }
}

struct AgentWorkRouter: Router {
    var basePath = ""
    static var generated: URL {
        FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0].appendingPathComponent("agentwork-modules", isDirectory: true)
    }

    func route(for path: String) -> String {
        FrontendCatalog.assetPath(root: URL(fileURLWithPath: basePath), generated: Self.generated, path: path)
    }
}

final class AgentWorkSchemeHandler: NSObject, WKURLSchemeHandler {
    private let fallback: WKURLSchemeHandler
    private let runtime: Result<RemoteRuntime, Error>
    private var pending: [ObjectIdentifier: String] = [:]

    init(fallback: WKURLSchemeHandler, runtime: Result<RemoteRuntime, Error>) {
        self.fallback = fallback
        self.runtime = runtime
    }

    func webView(_ webView: WKWebView, start task: WKURLSchemeTask) {
        guard let url = task.request.url else { task.didFailWithError(RemoteFailure.address); return }
        do {
            if URLComponents(url: url, resolvingAgainstBaseURL: false)?.path == "/plugins/" {
                let client = try runtime.get()
                let path = try client.catalog.prepareBundle(url.absoluteString)
                let bytes = try Data(contentsOf: AgentWorkRouter.generated.appendingPathComponent(URL(fileURLWithPath: path).lastPathComponent))
                task.didReceive(URLResponse(url: url, mimeType: "text/javascript", expectedContentLength: bytes.count, textEncodingName: "utf-8"))
                task.didReceive(bytes)
                task.didFinish()
            } else if url.path.hasPrefix("/api/") || url.path.hasPrefix("/stage/protyle/js/widget/resources/") {
                let client = try runtime.get()
                // Mutations use the proof-bearing bridge; WebKit's resource handler serves reads.
                guard ["GET", "HEAD"].contains(task.request.httpMethod ?? "GET") else { throw RemoteFailure.protocolData }
                var headers: [String: String] = task.request.allHTTPHeaderFields ?? [:]
                headers.removeValue(forKey: "Host")
                let request = try client.request(path: url.path + (url.query.map { "?" + $0 } ?? ""),
                                                 method: task.request.httpMethod ?? "GET", headers: headers, body: task.request.httpBody)
                let key = ObjectIdentifier(task)
                let id = "scheme-" + UUID().uuidString
                pending[key] = id
                try client.network.start(request, id: id) { event in
                    DispatchQueue.main.async {
                        guard self.pending[key] == id else { return }
                        switch event {
                        case .head(let response):
                            var fields: [String: String] = [:]
                            for (name, value) in response.allHeaderFields {
                                let key = String(describing: name).lowercased()
                                if !["set-cookie", "content-length", "content-encoding", "connection", "transfer-encoding"].contains(key) {
                                    fields[key] = String(describing: value)
                                }
                            }
                            task.didReceive(HTTPURLResponse(url: url, statusCode: response.statusCode, httpVersion: nil, headerFields: fields)!)
                        case .bytes(let bytes): task.didReceive(bytes)
                        case .end: self.pending.removeValue(forKey: key); task.didFinish()
                        case .failed(let error): self.pending.removeValue(forKey: key); task.didFailWithError(error)
                        }
                    }
                }
            } else { fallback.webView(webView, start: task) }
        } catch {
            pending.removeValue(forKey: ObjectIdentifier(task))
            let detail = error as NSError
            Logger(subsystem: "local.agentwork.ios", category: "resource").error("Local request failed in \(detail.domain, privacy: .public): \(detail.code)")
            task.didFailWithError(error)
        }
    }

    func webView(_ webView: WKWebView, stop task: WKURLSchemeTask) {
        if let id = pending.removeValue(forKey: ObjectIdentifier(task)) { try? runtime.get().network.cancel(id) }
        else { fallback.webView(webView, stop: task) }
    }
}
