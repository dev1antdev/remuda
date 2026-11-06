import AppKit

// DockBadge is the count on the Dock icon: the agents that need the person, nothing when none does.
enum DockBadge {
    static func set(count: Int) {
        NSApp.dockTile.badgeLabel = count > 0 ? String(count) : nil
    }
}
