// swift-tools-version: 6.0
import PackageDescription
let package = Package(name: "StageWandMac", platforms: [.macOS(.v14)], targets: [.executableTarget(name: "StageWandMac", path: "Sources")])
