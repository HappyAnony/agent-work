// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "AgentWorkNativeChecks",
    platforms: [.macOS(.v13)],
    targets: [
        .target(name: "AgentWorkCore", path: "ios/App/App/Native",
                exclude: ["AgentWorkPlugin.swift", "AgentWorkViewController.swift"],
                sources: ["RemoteModel.swift", "RemoteNetwork.swift", "RemoteRuntime.swift"]),
        .testTarget(name: "AgentWorkCoreTests", dependencies: ["AgentWorkCore"], path: "tests/native")
    ]
)
