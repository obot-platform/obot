// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "ObotSetup",
    platforms: [
        .macOS(.v13),
    ],
    products: [
        .executable(name: "ObotSetup", targets: ["ObotSetup"]),
    ],
    targets: [
        .executableTarget(
            name: "ObotSetup",
            path: "Sources/ObotSetup",
            resources: [
                .process("Resources"),
            ]
        ),
        .testTarget(
            name: "ObotSetupTests",
            dependencies: ["ObotSetup"],
            path: "Tests/ObotSetupTests"
        ),
    ]
)
