// Capacitor bridge for the packaged UI. No method returns the server credential.
import Capacitor
import Foundation
import WebKit
import OSLog

@objc(AgentWorkPlugin)
public final class AgentWorkPlugin: CAPPlugin, CAPBridgedPlugin {
    public let identifier = "AgentWorkPlugin"
    public let jsName = "AgentWork"
    public let pluginMethods: [CAPPluginMethod] = [
        CAPPluginMethod(name: "state", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "connect", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "localBootstrap", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "bootstrap", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "request", returnType: CAPPluginReturnCallback),
        CAPPluginMethod(name: "cancel", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "socket", returnType: CAPPluginReturnCallback),
        CAPPluginMethod(name: "send", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "close", returnType: CAPPluginReturnPromise),
        CAPPluginMethod(name: "failed", returnType: CAPPluginReturnPromise),
    ]
    var runtimeResult: Result<RemoteRuntime, Error> = .failure(RemoteFailure.resource)
    private let calls = NativeCallState()
    let admission = NativeAdmission()

    public override func shouldOverrideLoad(_ action: WKNavigationAction) -> NSNumber? {
        guard action.targetFrame?.isMainFrame == true, let url = action.request.url else { return nil }
        guard url.scheme == "dsh-app" else { return nil }
        guard url.host == "app", ["", "/", "/index.html", "/connect.html"].contains(url.path) else { return true }
        var next = URLComponents(url: url, resolvingAgainstBaseURL: false)
        var current = webView?.url.flatMap { URLComponents(url: $0, resolvingAgainstBaseURL: false) }
        next?.fragment = nil
        current?.fragment = nil
        if next?.string != current?.string || action.navigationType == .reload {
            calls.advance()
            runtimeResult.value?.network.reset()
        }
        return nil
    }

    @objc func state(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        respond(call) { ["serverUrl": try self.runtimeResult.get().savedAddress()] }
    }

    @objc func connect(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        guard let address = call.getString("serverUrl"), let token = call.getString("token"),
              let generation = calls.beginConnection() else {
            call.reject("invalid_request"); return
        }
        Task { @MainActor in
            defer { self.calls.finishConnection() }
            guard generation == self.calls.generation else { return }
            do {
                try await self.runtimeResult.get().connect(address: address, token: token)
                if generation == self.calls.generation { call.resolve() }
            } catch { if generation == self.calls.generation { call.reject(Self.failure(error)) } }
        }
    }

    @objc func localBootstrap(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        do { call.resolve(try runtimeResult.get().catalog.localBootstrap()) }
        catch { call.reject(Self.failure(error)) }
    }

    @objc func bootstrap(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        let generation = calls.generation
        Task { @MainActor in
            guard generation == self.calls.generation else { return }
            do {
                let response = try await self.runtimeResult.get().bootstrap()
                if generation == self.calls.generation { call.resolve(response) }
            } catch { if generation == self.calls.generation { call.reject(Self.failure(error)) } }
        }
    }

    @objc func request(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        do {
            let runtime = try runtimeResult.get()
            let generation = calls.generation
            let id = try identity(call)
            guard let path = call.getString("path"), let method = call.getString("method"),
                  let headers = call.getObject("headers") as? [String: String] else { throw RemoteFailure.protocolData }
            let body = call.getString("body").flatMap { Data(base64Encoded: $0) }
            if call.getString("body") != nil && body == nil { throw RemoteFailure.protocolData }
            let request = try runtime.request(path: path, method: method, headers: headers, body: body)
            call.keepAlive = true
            try runtime.network.start(request, id: id) { event in
                DispatchQueue.main.async {
                    guard generation == self.calls.generation else { call.keepAlive = false; return }
                    switch event {
                    case .head(let response):
                        let withheld = Set(["set-cookie", "content-length", "content-encoding", "transfer-encoding", "connection"])
                        var headers: [String: String] = [:]
                        for (name, value) in response.allHeaderFields {
                            let key = String(describing: name).lowercased()
                            if !withheld.contains(key) { headers[key] = String(describing: value) }
                        }
                        call.resolve(["type": "head", "status": response.statusCode, "headers": headers])
                    case .bytes(let data): call.resolve(["type": "data", "data": data.base64EncodedString()])
                    case .end: call.keepAlive = false; call.resolve(["type": "end"])
                    case .failed(let error): call.keepAlive = false; call.reject(Self.failure(error))
                    }
                }
            }
        } catch { call.keepAlive = false; call.reject(Self.failure(error)) }
    }

    @objc func cancel(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        respond(call) { self.runtimeResult.value?.network.cancel(try self.identity(call)); return [:] }
    }

    @objc func socket(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        do {
            let runtime = try runtimeResult.get()
            let generation = calls.generation
            let id = try identity(call)
            guard let url = call.getString("url"), let protocols = call.getArray("protocols", String.self) else { throw RemoteFailure.protocolData }
            let request = try runtime.socketRequest(url: url, protocols: protocols)
            call.keepAlive = true
            try runtime.network.open(request, id: id) { event in
                DispatchQueue.main.async {
                    guard generation == self.calls.generation else { call.keepAlive = false; return }
                    switch event {
                    case .open(let value): call.resolve(["type": "open", "protocol": value])
                    case .text(let value): call.resolve(["type": "message", "text": value])
                    case .binary(let value): call.resolve(["type": "message", "data": value.base64EncodedString()])
                    case .close(let code, let reason, let clean):
                        call.keepAlive = false
                        call.resolve(["type": "close", "code": code, "reason": reason, "clean": clean])
                    }
                }
            }
        } catch { call.keepAlive = false; call.reject(Self.failure(error)) }
    }

    @objc func send(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        let generation = calls.generation
        Task { @MainActor in
            guard generation == self.calls.generation else { return }
            do {
                let runtime = try self.runtimeResult.get()
                try await runtime.network.send(self.identity(call), text: call.getString("text"),
                                               bytes: call.getString("data").flatMap { Data(base64Encoded: $0) })
                if generation == self.calls.generation { call.resolve() }
            } catch { if generation == self.calls.generation { call.reject(Self.failure(error)) } }
        }
    }

    @objc func close(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        respond(call) {
            let reason = call.getString("reason") ?? ""
            let code = call.getInt("code") ?? 1000
            guard code == 1000 || (3000...4999).contains(code), reason.utf8.count <= 123 else { throw RemoteFailure.protocolData }
            try self.runtimeResult.get().network.close(self.identity(call), code: code, reason: reason)
            return [:]
        }
    }

    @objc func failed(_ call: CAPPluginCall) {
        guard admitted(call) else { return }
        let diagnostic = String((call.getString("message") ?? "client_startup").prefix(2048))
            .replacingOccurrences(of: "(?i)(https?|wss?)://\\S+", with: "<server>", options: .regularExpression)
        Logger(subsystem: "local.agentwork.ios", category: "startup").error("Frontend startup failed: \(diagnostic, privacy: .public)")
        runtimeResult.value?.network.reset()
        call.resolve()

    }

    func resume() { runtimeResult.value?.network.checkSockets() }
    func shutdown() { runtimeResult.value?.network.shutdown() }

    private func admitted(_ call: CAPPluginCall) -> Bool {
        guard admission.accepts(call.getString("nonce")) else { call.reject("invalid_source"); return false }
        return true
    }

    private func identity(_ call: CAPPluginCall) throws -> String {
        guard let id = call.getString("id"), id.range(of: "^[a-zA-Z0-9_-]{1,100}$", options: .regularExpression) != nil else {
            throw RemoteFailure.protocolData
        }
        return id
    }
    private func respond(_ call: CAPPluginCall, operation: () throws -> JSObject) {
        do { call.resolve(try operation()) }
        catch { call.reject(Self.failure(error)) }
    }
    private static func failure(_ error: Error) -> String {
        let detail = error as NSError
        Logger(subsystem: "local.agentwork.ios", category: "network").error("Operation failed in \(detail.domain, privacy: .public): \(detail.code)")
        if let failure = error as? RemoteFailure { return failure.rawValue }
        if detail.domain == NSCocoaErrorDomain && [NSFileReadNoSuchFileError, NSFileNoSuchFileError].contains(detail.code) { return RemoteFailure.resource.rawValue }
        if let network = error as? URLError {
            if network.code == .cancelled { return "cancelled" }
            if [.serverCertificateUntrusted, .serverCertificateHasBadDate, .serverCertificateHasUnknownRoot, .serverCertificateNotYetValid, .secureConnectionFailed].contains(network.code) { return "certificate" }
        }
        return "network"
    }
}

private extension Result {
    var value: Success? { try? get() }
}
