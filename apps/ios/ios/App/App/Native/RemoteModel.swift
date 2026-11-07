// Packaged resource verification and authority-bound credentials for the iOS carrier.
import Foundation
import CryptoKit
import Security

enum RemoteFailure: String, Error {
    case address = "server_address"
    case authentication = "authentication"
    case version = "frontend_version"
    case resource = "frontend_resource"
    case limit = "transfer_limit"
    case protocolData = "invalid_response"
    case credential = "credential_storage"
    case disconnected = "disconnected"
}

// The proof is injected only into local application documents, never remote widgets.
struct NativeAdmission {
    let nonce = UUID().uuidString
    func accepts(_ proof: String?) -> Bool { proof == nonce }
}

// Capacitor invokes methods on its bridge queue; navigation and completions run on main.
final class NativeCallState {
    private let lock = NSLock()
    private var value: UInt64 = 0
    private var connecting = false
    var generation: UInt64 { lock.lock(); defer { lock.unlock() }; return value }
    func advance() { lock.lock(); defer { lock.unlock() }; value &+= 1 }
    func beginConnection() -> UInt64? {
        lock.lock(); defer { lock.unlock() }
        guard !connecting else { return nil }
        connecting = true
        return value
    }
    func finishConnection() { lock.lock(); defer { lock.unlock() }; connecting = false }
}

struct RemoteAuthority: Codable, Equatable {
    let origin: String

    init(_ input: String) throws {
        guard var parts = URLComponents(string: input), parts.scheme?.lowercased() == "https",
              let host = parts.host, !host.isEmpty, parts.user == nil, parts.password == nil,
              parts.path.isEmpty || parts.path == "/", parts.query == nil, parts.fragment == nil,
              parts.port.map({ (1...65535).contains($0) }) ?? true else { throw RemoteFailure.address }
        parts.path = ""
        parts.scheme = "https"
        parts.host = host.lowercased()
        if parts.port == 443 { parts.port = nil }
        guard let value = parts.string else { throw RemoteFailure.address }
        origin = value
    }

    func endpoint(_ path: String) throws -> URL {
        guard path.hasPrefix("/"), !path.hasPrefix("//"),
              let value = URL(string: origin + path), let parts = URLComponents(url: value, resolvingAgainstBaseURL: false),
              let base = URLComponents(string: origin), parts.scheme == base.scheme,
              parts.host == base.host, parts.port == base.port, parts.user == nil, parts.password == nil,
              parts.fragment == nil else { throw RemoteFailure.address }
        return value
    }

    func websocket(_ input: String) throws -> URL {
        guard let incoming = URLComponents(string: input), let expected = URLComponents(string: origin),
              incoming.scheme == "wss", incoming.host == expected.host, incoming.port == expected.port,
              incoming.user == nil, incoming.password == nil, incoming.fragment == nil,
              let url = incoming.url else { throw RemoteFailure.address }
        switch incoming.path {
        case "/api/remote.mux":
            guard incoming.query == nil else { throw RemoteFailure.address }
        case "/api/wikiman/v1/events":
            let items = incoming.queryItems ?? []
            guard items.count == 2, Set(items.map { $0.name }) == Set(["storeId", "pageId"]),
                  items.allSatisfy({ $0.value?.range(of: "^[A-Za-z0-9-]{1,100}$", options: .regularExpression) != nil }) else {
                throw RemoteFailure.address
            }
        default: throw RemoteFailure.address
        }
        return url
    }
}

struct RemoteCredential: Codable {
    let authority: RemoteAuthority
    let cookie: String
}

final class CredentialVault {
    private let service: String
    init(service: String) { self.service = service }

    func load() throws -> RemoteCredential? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess, let bytes = result as? Data else { throw RemoteFailure.credential }
        let value = try JSONDecoder().decode(RemoteCredential.self, from: bytes)
        _ = try RemoteAuthority(value.authority.origin)
        return value
    }

    func save(_ credential: RemoteCredential) throws {
        let bytes = try JSONEncoder().encode(credential)
        let attributes = [kSecValueData as String: bytes]
        var status = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            var item = baseQuery
            item[kSecValueData as String] = bytes
            item[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            status = SecItemAdd(item as CFDictionary, nil)
        }
        guard status == errSecSuccess else { throw RemoteFailure.credential }
    }

    private var baseQuery: [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service,
         kSecAttrAccount as String: "remote-server"]
    }
}

struct FrontendManifest: Decodable {
    let protocolVersion: Int
    let frontendHash: String
    let bundles: [String: String]
    let wikiHash: String
}

final class FrontendCatalog {
    let root: URL
    let generated: URL
    private let manifest: FrontendManifest

    init(root: URL, generated: URL) throws {
        self.root = root
        self.generated = generated
        manifest = try JSONDecoder().decode(FrontendManifest.self, from: Data(contentsOf: root.appendingPathComponent("manifest.json")))
        guard manifest.protocolVersion == 1,
              Self.digest(try Data(contentsOf: root.appendingPathComponent("_ios/original-index.html"))) == manifest.frontendHash else {
            throw RemoteFailure.resource
        }
        try FileManager.default.createDirectory(at: generated, withIntermediateDirectories: true)
    }

    func verify(_ bootstrap: [String: Any]) throws {
        guard let version = bootstrap["protocolVersion"] as? NSNumber,
              CFGetTypeID(version) != CFBooleanGetTypeID(), version.doubleValue == 1,
              bootstrap["frontendHash"] as? String == manifest.frontendHash,
              let bundles = bootstrap["bundles"] as? [String: String],
              let injections = bootstrap["injections"] as? [[String: Any]] else { throw RemoteFailure.version }
        for (name, hash) in bundles {
            guard manifest.bundles[name] == hash, Self.validPackage(name),
                  Self.digest(try Data(contentsOf: root.appendingPathComponent("_ios/plugins/\(name)/client.js"))) == hash else {
                throw RemoteFailure.version
            }
        }
        if let wiki = injections.first(where: { $0["kind"] as? String == "global" && $0["name"] as? String == "__DSH_WIKIMAN_ASSETS__" }) {
            guard wiki["value"] as? String == manifest.wikiHash,
                  Self.digest(try Data(contentsOf: root.appendingPathComponent("stage/build/portable/boot.js"))) == manifest.wikiHash else {
                throw RemoteFailure.version
            }
        }
    }

    func prepareBundle(_ input: String) throws -> String {
        guard let parts = URLComponents(string: input), parts.scheme == nil || parts.scheme == "dsh-app" && parts.host == "app",
              parts.path == "/plugins/", let query = parts.query,
              query.hasPrefix("?"), let encoded = query.dropFirst().split(separator: "&", maxSplits: 1).first,
              !encoded.isEmpty else { throw RemoteFailure.resource }
        let entries = String(encoded)
        var bytes = Data()
        for entry in entries.split(separator: ",") {
            let suffix = "/client.js"
            guard entry.hasSuffix(suffix) else { throw RemoteFailure.resource }
            let name = String(entry.dropLast(suffix.count))
            guard Self.validPackage(name), let hash = manifest.bundles[name] else { throw RemoteFailure.resource }
            let source = try Data(contentsOf: root.appendingPathComponent("_ios/plugins/\(name)/client.js"))
            guard Self.digest(source) == hash, let script = String(data: source, encoding: .utf8) else { throw RemoteFailure.resource }
            let cleaned = script.replacingOccurrences(of: "(?m)//[#@] sourceMappingURL=[^\\n]*(?:\\n|$)", with: "", options: .regularExpression)
            bytes.append(Data((cleaned + "\n;\n").utf8))
            guard bytes.count <= 64 * 1024 * 1024 else { throw RemoteFailure.limit }
        }
        let name = Self.digest(bytes) + ".js"
        let destination = generated.appendingPathComponent(name)
        if FileManager.default.fileExists(atPath: destination.path) {
            if Self.digest(try Data(contentsOf: destination)) != Self.digest(bytes) { try bytes.write(to: destination, options: .atomic) }
        } else { try bytes.write(to: destination, options: .atomic) }
        return "/_ios/modules/" + name
    }

    static func digest(_ data: Data) -> String { SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined() }

    static func assetPath(root: URL, generated: URL, path: String) -> String {
        if path.range(of: "^/_ios/modules/[a-f0-9]{64}\\.js$", options: .regularExpression) != nil {
            return generated.appendingPathComponent(URL(fileURLWithPath: path).lastPathComponent).path
        }
        let local = path.hasPrefix("/plugins/@") ? "/_ios" + path : path
        let base = root.standardizedFileURL
        var target = base.appendingPathComponent(local).standardizedFileURL
        guard target.path == base.path || target.path.hasPrefix(base.path + "/") else { return base.appendingPathComponent("missing-resource").path }
        var directory: ObjCBool = false
        if FileManager.default.fileExists(atPath: target.path, isDirectory: &directory), directory.boolValue {
            target.appendPathComponent("index.html")
        }
        return target.path
    }

    private static func validPackage(_ name: String) -> Bool {
        name.range(of: "^@[a-z0-9._-]+/[a-z0-9._-]+$", options: .regularExpression) != nil
    }
}
