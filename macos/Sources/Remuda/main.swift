import AppKit

// The entry: `--probe` runs the headless check and exits, anything else is the app with its window and menu bar item.
if let probe = Probe.parse(CommandLine.arguments) {
    exit(probe.run())
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
