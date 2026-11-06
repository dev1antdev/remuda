// swift-tools-version:5.9
// The macOS shell for remuda's page: a window, the menu bar, native notifications. System frameworks only.
import PackageDescription

let package = Package(
    name: "Remuda",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "Remuda",
            path: "Sources/Remuda",
            linkerSettings: [
                .linkedFramework("AppKit"),
                .linkedFramework("WebKit"),
                .linkedFramework("UserNotifications"),
                .linkedFramework("Carbon"),
                .linkedFramework("ServiceManagement"),
                .linkedFramework("Security"),
            ]
        ),
    ]
)
