import XCTest
@testable import AgentWorkCore

final class ResourcesTests: XCTestCase {
    func testServerSelectionRejectsNonRootAndUnencryptedAddresses() throws {
        XCTAssertEqual(try RemoteAuthority("https://example.test:18686/").origin, "https://example.test:18686")
        let canonical = try RemoteAuthority("HTTPS://EXAMPLE.TEST:443/")
        XCTAssertEqual(canonical.origin, "https://example.test")
        XCTAssertNoThrow(try canonical.websocket("wss://example.test/api/remote.mux"))
        for value in ["http://example.test", "https://user@example.test", "https://example.test/wiki", "https://example.test?token=x", "https://example.test#x", "https://example.test:0"] {
            XCTAssertThrowsError(try RemoteAuthority(value), value)
        }
        let authority = try RemoteAuthority("https://example.test:18686")
        XCTAssertEqual(try authority.endpoint("/api/rpc?test=1").absoluteString, "https://example.test:18686/api/rpc?test=1")
        for value in ["//other.test/api/rpc", "https://other.test/api/rpc", "/api/rpc#fragment"] {
            XCTAssertThrowsError(try authority.endpoint(value), value)
        }
        XCTAssertNoThrow(try authority.websocket("wss://example.test:18686/api/remote.mux"))
        XCTAssertNoThrow(try authority.websocket("wss://example.test:18686/api/wikiman/v1/events?storeId=store-1&pageId=1234567890123456789"))
        for value in ["wss://other.test:18686/api/remote.mux", "ws://example.test:18686/api/remote.mux", "wss://example.test:18686/api/remote.mux?token=secret", "wss://example.test:18686/api/wikiman/v1/events?storeId=a&storeId=b", "wss://example.test:18686/api/wikiman/v1/events?storeId=a&pageId=b&token=secret"] {
            XCTAssertThrowsError(try authority.websocket(value), value)
        }
    }

    func testLocalBundleVerificationRejectsVersionMismatchAndModifiedFiles() throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: root) }
        func put(_ path: String, _ bytes: Data) throws {
            let url = root.appendingPathComponent(path)
            try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
            try bytes.write(to: url)
        }
        let index = Data("<html></html>".utf8), js = Data("globalThis.plugin = true;\n".utf8), wiki = Data("wiki".utf8)
        let modules = ["@test/plugin": FrontendCatalog.digest(js)]
        let manifest: [String: Any] = ["protocolVersion": 1, "frontendHash": FrontendCatalog.digest(index), "bundles": modules, "wikiHash": FrontendCatalog.digest(wiki)]
        try put("manifest.json", JSONSerialization.data(withJSONObject: manifest))
        try put("_ios/original-index.html", index)
        try put("_ios/plugins/@test/plugin/client.js", js)
        try put("stage/build/portable/boot.js", wiki)
        try put("stage/build/portable/index.html", index)
        let catalog = try FrontendCatalog(root: root, generated: root.appendingPathComponent("generated"))
        XCTAssertEqual(FrontendCatalog.assetPath(root: root, generated: catalog.generated, path: "/stage/build/portable"), root.appendingPathComponent("stage/build/portable/index.html").path)
        XCTAssertEqual(FrontendCatalog.assetPath(root: root, generated: catalog.generated, path: "/plugins/@test/plugin/client.js"), root.appendingPathComponent("_ios/plugins/@test/plugin/client.js").path)
        var bootstrap = manifest
        bootstrap["injections"] = [["kind": "global", "name": "__DSH_WIKIMAN_ASSETS__", "value": FrontendCatalog.digest(wiki)]]
        try put("bootstrap.json", JSONSerialization.data(withJSONObject: bootstrap))
        XCTAssertEqual(try catalog.localBootstrap()["frontendHash"] as? String, FrontendCatalog.digest(index))
        XCTAssertNoThrow(try catalog.verify(bootstrap))
        XCTAssertTrue(try catalog.prepareBundle("/plugins/??@test/plugin/client.js").hasPrefix("/_ios/modules/"))
        XCTAssertNoThrow(try catalog.prepareBundle("dsh-app://app/plugins/?%3F%40test%2Fplugin%2Fclient.js&rev=fixture"))
        for path in ["/plugins/??../client.js", "dsh-app://other/plugins/??@test/plugin/client.js", "/plugins/??@test/absent/client.js"] {
            XCTAssertThrowsError(try catalog.prepareBundle(path), path)
        }
        var wrong = bootstrap
        wrong["protocolVersion"] = true
        XCTAssertThrowsError(try catalog.verify(wrong))
        wrong = bootstrap; wrong["frontendHash"] = "old"
        XCTAssertThrowsError(try catalog.verify(wrong))
        try put("_ios/plugins/@test/plugin/client.js", Data("modified".utf8))
        XCTAssertThrowsError(try catalog.verify(bootstrap))
        XCTAssertThrowsError(try catalog.prepareBundle("/plugins/??@test/plugin/client.js"))
        try put("_ios/original-index.html", Data("modified".utf8))
        XCTAssertThrowsError(try FrontendCatalog(root: root, generated: root.appendingPathComponent("generated")))
    }
}
