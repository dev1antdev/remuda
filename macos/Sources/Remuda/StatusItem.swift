import AppKit

// StatusItem is the menu bar item: the mark with a tone dot and the count, and the menu of agents that need the person.
final class StatusItem: NSObject {
    private let item: NSStatusItem
    private let menu = NSMenu()
    private var rows: [MenuRow] = []
    private var working = 0
    private var idle = 0
    private var heard = false
    var hotkeyLabel = "⌥⌘R"
    var onSelect: ((MenuRow) -> Void)?
    var onOpen: (() -> Void)?
    var onMode: ((String) -> Void)?
    var onPanel: ((String) -> Void)?
    var onSettings: (() -> Void)?

    override init() {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        super.init()
        menu.autoenablesItems = false
        item.menu = menu
        item.button?.imagePosition = .imageLeading
        item.button?.image = StatusMark.image(tone: nil)
        rebuild()
    }

    // setBadge draws the count and the tone of the most pressing agent on the item.
    func setBadge(count: Int, tone: Tone?) {
        item.button?.image = StatusMark.image(tone: tone)
        item.button?.title = count > 0 ? String(count) : ""
    }

    // setMenu keeps the page's reading of who needs the person and redraws the menu.
    func setMenu(rows: [MenuRow], working: Int, idle: Int) {
        self.rows = rows
        self.working = working
        self.idle = idle
        heard = true
        rebuild()
    }

    private func rebuild() {
        menu.removeAllItems()
        menu.addItem(disabled(headline))
        for (index, row) in rows.enumerated() {
            let entry = NSMenuItem(title: row.title, action: #selector(pick(_:)), keyEquivalent: "")
            entry.target = self
            entry.tag = index
            entry.attributedTitle = StatusItem.title(of: row)
            entry.toolTip = row.said.isEmpty ? nil : row.said
            menu.addItem(entry)
        }
        if heard {
            menu.addItem(disabled("\(working) working · \(idle) idle"))
        }
        menu.addItem(.separator())
        let open = NSMenuItem(title: "Open remuda", action: #selector(openPage), keyEquivalent: "r")
        open.keyEquivalentModifierMask = [.option, .command]
        open.target = self
        menu.addItem(open)
        for (title, selector) in [("Wall", #selector(wall)), ("Tasks", #selector(tasks)), ("Standup", #selector(standup))] {
            let entry = NSMenuItem(title: title, action: selector, keyEquivalent: "")
            entry.target = self
            menu.addItem(entry)
        }
        menu.addItem(.separator())
        let settings = NSMenuItem(title: "Settings…", action: #selector(openSettings), keyEquivalent: ",")
        settings.target = self
        menu.addItem(settings)
        let quit = NSMenuItem(title: "Quit remuda", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)
    }

    private var headline: String {
        if !heard { return "Waiting for the page…" }
        switch rows.count {
        case 0: return "Nobody needs you"
        case 1: return "1 agent needs you"
        default: return "\(rows.count) agents need you"
        }
    }

    private func disabled(_ title: String) -> NSMenuItem {
        let entry = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        entry.isEnabled = false
        return entry
    }

    // title lays a row out as the glyph in its tone, the agent and its machine, and what it said on a second quieter line.
    private static func title(of row: MenuRow) -> NSAttributedString {
        let glyph: String
        let colour: NSColor
        switch row.tone {
        case .red: (glyph, colour) = ("?", .systemRed)
        case .amber: (glyph, colour) = ("!", .systemOrange)
        case .green: (glyph, colour) = ("✓", .systemGreen)
        case nil: (glyph, colour) = ("·", .secondaryLabelColor)
        }
        let text = NSMutableAttributedString(string: glyph, attributes: [.font: NSFont.menuFont(ofSize: 0), .foregroundColor: colour])
        text.append(NSAttributedString(string: " \(row.title) · \(row.machineLabel)", attributes: [.font: NSFont.menuFont(ofSize: 0), .foregroundColor: NSColor.labelColor]))
        if !row.said.isEmpty {
            let said = row.said.count > 72 ? String(row.said.prefix(71)) + "…" : row.said
            text.append(NSAttributedString(string: "\n   " + said, attributes: [.font: NSFont.menuFont(ofSize: NSFont.smallSystemFontSize), .foregroundColor: NSColor.secondaryLabelColor]))
        }
        return text
    }

    @objc private func pick(_ sender: NSMenuItem) {
        guard rows.indices.contains(sender.tag) else { return }
        onSelect?(rows[sender.tag])
    }

    @objc private func openPage() { onOpen?() }
    @objc private func wall() { onMode?("wall") }
    @objc private func tasks() { onMode?("tasks") }
    @objc private func standup() { onPanel?("standup") }
    @objc private func openSettings() { onSettings?() }
}

// StatusMark draws the menu bar image: a monospaced "r" in a rounded square, and a dot in the tone's colour when there is one.
enum StatusMark {
    static func image(tone: Tone?) -> NSImage {
        let image = NSImage(size: NSSize(width: 18, height: 18), flipped: false) { rect in
            let ink = NSColor.labelColor
            let box = NSBezierPath(roundedRect: rect.insetBy(dx: 1.5, dy: 1.5), xRadius: 4, yRadius: 4)
            box.lineWidth = 1.5
            ink.setStroke()
            box.stroke()
            let letter = NSAttributedString(string: "r", attributes: [.font: NSFont.monospacedSystemFont(ofSize: 11, weight: .bold), .foregroundColor: ink])
            let size = letter.size()
            letter.draw(at: NSPoint(x: (rect.width - size.width) / 2, y: (rect.height - size.height) / 2 + 0.5))
            if let tone {
                let dot = NSBezierPath(ovalIn: NSRect(x: rect.maxX - 7, y: rect.minY, width: 7, height: 7))
                colour(of: tone).setFill()
                dot.fill()
                ink.withAlphaComponent(0.2).setStroke()
                dot.lineWidth = 0.5
                dot.stroke()
            }
            return true
        }
        image.isTemplate = tone == nil
        return image
    }

    static func colour(of tone: Tone) -> NSColor {
        switch tone {
        case .red: return .systemRed
        case .amber: return .systemOrange
        case .green: return .systemGreen
        }
    }
}
