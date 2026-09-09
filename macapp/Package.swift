// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "SchoolSub2APIMac",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(name: "SchoolSub2APIMac", targets: ["SchoolSub2APIMac"])
    ],
    targets: [
        .executableTarget(
            name: "SchoolSub2APIMac",
            path: "Sources/SchoolSub2APIMac"
        ),
        .testTarget(
            name: "SchoolSub2APIMacTests",
            dependencies: ["SchoolSub2APIMac"],
            path: "Tests/SchoolSub2APIMacTests"
        )
    ]
)
