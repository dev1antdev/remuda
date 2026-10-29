import AppKit
import Carbon

// Hotkey is one global key combination, registered with Carbon so it works whichever app is in front.
final class Hotkey {
    static let defaultSpec = "opt+cmd+r"
    private var reference: EventHotKeyRef?
    private var handler: EventHandlerRef?
    private let action: () -> Void

    init?(spec: String, action: @escaping () -> Void) {
        guard let (code, modifiers) = Hotkey.parse(spec) else { return nil }
        self.action = action
        var kind = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        let me = Unmanaged.passUnretained(self).toOpaque()
        InstallEventHandler(GetApplicationEventTarget(), { _, _, data in
            guard let data else { return noErr }
            Unmanaged<Hotkey>.fromOpaque(data).takeUnretainedValue().action()
            return noErr
        }, 1, &kind, me, &handler)
        let id = EventHotKeyID(signature: OSType(0x524D_4441), id: 1)
        if RegisterEventHotKey(code, modifiers, id, GetApplicationEventTarget(), 0, &reference) != noErr {
            if let handler { RemoveEventHandler(handler) }
            handler = nil
            return nil
        }
    }

    deinit {
        if let reference { UnregisterEventHotKey(reference) }
        if let handler { RemoveEventHandler(handler) }
    }

    private static let keyCodes: [String: UInt32] = [
        "a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8, "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15,
        "y": 16, "t": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "9": 25, "7": 26, "8": 28, "0": 29, "o": 31, "u": 32,
        "i": 34, "p": 35, "l": 37, "j": 38, "k": 40, "n": 45, "m": 46, "space": 49,
    ]

    // parse reads "opt+cmd+r" or "⌥⌘R": cmd/⌘, opt/alt/⌥, ctrl/⌃, shift/⇧ and one letter, digit or space.
    static func parse(_ spec: String) -> (UInt32, UInt32)? {
        var modifiers: UInt32 = 0
        var key = ""
        let cleaned = spec.lowercased().replacingOccurrences(of: "⌘", with: "cmd+").replacingOccurrences(of: "⌥", with: "opt+")
            .replacingOccurrences(of: "⌃", with: "ctrl+").replacingOccurrences(of: "⇧", with: "shift+").replacingOccurrences(of: " ", with: "")
        for part in cleaned.split(separator: "+").map(String.init) where !part.isEmpty {
            switch part {
            case "cmd", "command": modifiers |= UInt32(cmdKey)
            case "opt", "alt", "option": modifiers |= UInt32(optionKey)
            case "ctrl", "control": modifiers |= UInt32(controlKey)
            case "shift": modifiers |= UInt32(shiftKey)
            default: key = part
            }
        }
        guard modifiers != 0, let code = keyCodes[key] else { return nil }
        return (code, modifiers)
    }

    // label is the spec as a menu shows it, "⌥⌘R".
    static func label(_ spec: String) -> String {
        guard let (_, modifiers) = parse(spec) else { return spec }
        var out = ""
        if modifiers & UInt32(controlKey) != 0 { out += "⌃" }
        if modifiers & UInt32(optionKey) != 0 { out += "⌥" }
        if modifiers & UInt32(shiftKey) != 0 { out += "⇧" }
        if modifiers & UInt32(cmdKey) != 0 { out += "⌘" }
        let key = spec.lowercased().split(separator: "+").last.map(String.init) ?? ""
        return out + (key == "space" ? "Space" : key.uppercased())
    }
}
