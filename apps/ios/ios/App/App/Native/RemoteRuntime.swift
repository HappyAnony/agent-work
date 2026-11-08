// One selected server, with credentials retained only by the native process and Keychain.
import Foundation

final class RemoteRuntime {
    let network = RemoteNetwork()
    let catalog: FrontendCatalog
    private let vault: CredentialVault
    private let selectionKey: String
    private let lock = NSLock()
    private var credential: RemoteCredential?

    init(root: URL, generated: URL, service: String) throws {
        catalog = try FrontendCatalog(root: root, generated: generated)
        vault = CredentialVault(service: service)
        selectionKey = service + ".selected-server"
    }

    func savedAddress() throws -> String {
        if let selected = UserDefaults.standard.string(forKey: selectionKey) { return selected }
        return try vault.load()?.authority.origin ?? ""
    }

    func connect(address: String, token: String) async throws {
        let authority = try RemoteAuthority(address)
        UserDefaults.standard.set(authority.origin, forKey: selectionKey)
        network.reset()
        setCredential(nil)
        let next: RemoteCredential
        if token.isEmpty {
            guard let saved = try vault.load(), saved.authority == authority else { throw RemoteFailure.authentication }
            next = saved
        } else {
            guard token.utf8.count <= 4096, !token.contains("\r"), !token.contains("\n") else { throw RemoteFailure.authentication }
            var url = URLComponents(url: try authority.endpoint("/"), resolvingAgainstBaseURL: false)!
            url.queryItems = [URLQueryItem(name: "token", value: token)]
            let (_, response) = try await network.read(URLRequest(url: url.url!), limit: 8192)
            guard response.statusCode == 303, let header = response.value(forHTTPHeaderField: "set-cookie"),
                  let cookie = header.split(separator: ";", maxSplits: 1).first,
                  cookie.contains("="), cookie.utf8.count <= 8192, !cookie.contains("\r"), !cookie.contains("\n") else {
                throw RemoteFailure.authentication
            }
            next = RemoteCredential(authority: authority, cookie: String(cookie))
        }
        _ = try await readBootstrap(next)
        try vault.save(next)
        network.reset()
        setCredential(next)
    }

    func bootstrap() async throws -> [String: Any] {
        let credential = try activeCredential()
        var bootstrap = try await readBootstrap(credential)
        bootstrap["streamBaseUrl"] = credential.authority.origin
        return bootstrap
    }

    private func readBootstrap(_ credential: RemoteCredential) async throws -> [String: Any] {
        var request = URLRequest(url: try credential.authority.endpoint("/api/client/bootstrap"))
        request.setValue(credential.cookie, forHTTPHeaderField: "cookie")
        let (data, response) = try await network.read(request, limit: 8 * 1024 * 1024)
        guard response.statusCode == 200 else { throw RemoteFailure.authentication }
        guard let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] else { throw RemoteFailure.protocolData }
        try catalog.verify(object)
        return object
    }

    func request(path: String, method: String, headers: [String: String], body: Data?) throws -> URLRequest {
        let credential = try activeCredential()
        if let origin = headers.first(where: { $0.key.lowercased() == "origin" })?.value,
           origin != "dsh-app://app" { throw RemoteFailure.address }
        guard path.hasPrefix("/api/") || path.hasPrefix("/stage/protyle/js/widget/resources/"),
              ["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"].contains(method),
              (body?.count ?? 0) <= 32 * 1024 * 1024 else { throw RemoteFailure.protocolData }
        var request = URLRequest(url: try credential.authority.endpoint(path))
        request.httpMethod = method
        request.httpBody = body
        let withheld = Set(["host", "origin", "cookie", "authorization", "proxy-authorization", "connection", "sec-fetch-site", "content-length"])
        for (name, value) in headers where !withheld.contains(name.lowercased()) {
            guard !name.contains("\r"), !name.contains("\n"), !value.contains("\r"), !value.contains("\n") else { throw RemoteFailure.protocolData }
            request.setValue(value, forHTTPHeaderField: name)
        }
        request.setValue(credential.cookie, forHTTPHeaderField: "cookie")
        return request
    }

    func socketRequest(url: String, protocols: [String]) throws -> URLRequest {
        let credential = try activeCredential()
        let target = try credential.authority.websocket(url)
        guard protocols.count <= 4,
              protocols.allSatisfy({ !$0.isEmpty && $0.utf8.count <= 4096 && $0.range(of: "^[A-Za-z0-9._~-]+$", options: .regularExpression) != nil }) else {
            throw RemoteFailure.address
        }
        var request = URLRequest(url: target)
        request.setValue(credential.cookie, forHTTPHeaderField: "cookie")
        request.setValue(credential.authority.origin, forHTTPHeaderField: "Origin")
        if !protocols.isEmpty { request.setValue(protocols.joined(separator: ", "), forHTTPHeaderField: "Sec-WebSocket-Protocol") }
        return request
    }

    private func activeCredential() throws -> RemoteCredential {
        lock.lock(); defer { lock.unlock() }
        guard let value = credential else { throw RemoteFailure.disconnected }
        return value
    }
    private func setCredential(_ value: RemoteCredential?) {
        lock.lock(); defer { lock.unlock() }
        credential = value
    }
}
