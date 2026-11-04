// Prints web/src/features/icons/emojiData.ts, the folder-icon picker's emoji: every scalar the Swift standard
// library's Unicode data marks as emoji and the system font "Apple Color Emoji" draws as one glyph, plus every
// region's flag the font draws as one ligature, each with its Unicode name, in nine groups by code point.
// Run on macOS from the repository root: swift web/scripts/emoji.swift > web/src/features/icons/emojiData.ts

import CoreText
import Foundation

let emojiFont = CTFontCreateWithName("AppleColorEmoji" as CFString, 16, nil)

// drawnAsOne says whether the emoji font draws the text as a single glyph of its own, with no fallback font
func drawnAsOne(_ text: String) -> Bool {
  let attributed = NSAttributedString(string: text, attributes: [NSAttributedString.Key(kCTFontAttributeName as String): emojiFont])
  let line = CTLineCreateWithAttributedString(attributed)
  guard CTLineGetGlyphCount(line) == 1, let run = (CTLineGetGlyphRuns(line) as? [CTRun])?.first else { return false }
  let attributes = CTRunGetAttributes(run) as NSDictionary
  guard let font = attributes[kCTFontAttributeName as String] else { return false }
  guard CTFontCopyPostScriptName(font as! CTFont) as String == "AppleColorEmoji" else { return false }
  var glyph = CGGlyph(0)
  CTRunGetGlyphs(run, CFRange(location: 0, length: 1), &glyph)
  return glyph != 0
}

struct Group {
  let id: String
  let name: String
  var emoji: [(String, String)] = []
}

var groups = [
  Group(id: "smileys", name: "Smileys"),
  Group(id: "people", name: "People"),
  Group(id: "animals", name: "Animals & nature"),
  Group(id: "food", name: "Food & drink"),
  Group(id: "travel", name: "Travel & places"),
  Group(id: "activities", name: "Activities"),
  Group(id: "objects", name: "Objects"),
  Group(id: "symbols", name: "Symbols"),
  Group(id: "flags", name: "Flags"),
]

// rules sort a code point into a group, the first range that holds it winning: an approximate table after the
// Unicode blocks and CLDR's emoji groups, not CLDR's own ordering
let rules: [(ClosedRange<UInt32>, String)] = [
  // flags that are single scalars
  (0x1F38C...0x1F38C, "flags"), (0x1F3C1...0x1F3C1, "flags"), (0x1F3F3...0x1F3F4, "flags"), (0x1F6A9...0x1F6A9, "flags"),
  // smileys: faces, emotions, hearts, speech
  (0x263A...0x263A, "smileys"), (0x2639...0x2639, "smileys"), (0x2763...0x2764, "smileys"),
  (0x1F479...0x1F480, "smileys"), (0x1F48B...0x1F48C, "smileys"), (0x1F493...0x1F49F, "smileys"),
  (0x1F4A2...0x1F4A6, "smileys"), (0x1F4A8...0x1F4A9, "smileys"), (0x1F4AB...0x1F4AD, "smileys"),
  (0x1F4AC...0x1F4AD, "smileys"), (0x1F573...0x1F573, "smileys"), (0x1F5A4...0x1F5A4, "smileys"),
  (0x1F5E8...0x1F5E8, "smileys"), (0x1F5EF...0x1F5EF, "smileys"), (0x1F600...0x1F644, "smileys"),
  (0x1F648...0x1F64A, "smileys"), (0x1F90D...0x1F90E, "smileys"), (0x1F910...0x1F917, "smileys"),
  (0x1F920...0x1F925, "smileys"), (0x1F927...0x1F92F, "smileys"), (0x1F970...0x1F97A, "smileys"),
  (0x1F9D0...0x1F9D0, "smileys"), (0x1F9E1...0x1F9E1, "smileys"), (0x1FA75...0x1FA77, "smileys"),
  (0x1FAE0...0x1FAEF, "smileys"),
  // people: hands, body, people, clothing
  (0x261D...0x261D, "people"), (0x26F7...0x26F7, "people"), (0x26F9...0x26F9, "people"), (0x270A...0x270D, "people"),
  (0x1F385...0x1F385, "people"), (0x1F393...0x1F393, "people"), (0x1F3A9...0x1F3A9, "people"),
  (0x1F3C2...0x1F3C4, "people"), (0x1F3C7...0x1F3C7, "people"), (0x1F3CA...0x1F3CC, "people"),
  (0x1F440...0x1F478, "people"), (0x1F481...0x1F487, "people"), (0x1F48F...0x1F48F, "people"),
  (0x1F491...0x1F491, "people"), (0x1F4AA...0x1F4AA, "people"), (0x1F574...0x1F57A, "people"),
  (0x1F590...0x1F596, "people"), (0x1F645...0x1F647, "people"), (0x1F64B...0x1F64F, "people"),
  (0x1F6A3...0x1F6A3, "people"), (0x1F6B4...0x1F6B6, "people"), (0x1F6C0...0x1F6C0, "people"),
  (0x1F6CC...0x1F6CC, "people"), (0x1F90C...0x1F90C, "people"), (0x1F90F...0x1F90F, "people"),
  (0x1F918...0x1F91F, "people"), (0x1F926...0x1F926, "people"), (0x1F930...0x1F939, "people"),
  (0x1F93C...0x1F93E, "people"), (0x1F97B...0x1F97F, "people"), (0x1F9B4...0x1F9B9, "people"),
  (0x1F9BB...0x1F9BB, "people"), (0x1F9BE...0x1F9BF, "people"), (0x1F9CD...0x1F9CF, "people"),
  (0x1F9D1...0x1F9DF, "people"), (0x1F9E2...0x1F9E6, "people"), (0x1FA70...0x1FA74, "people"),
  (0x1FAAE...0x1FAAE, "people"), (0x1FAC0...0x1FAC5, "people"), (0x1FAF0...0x1FAF8, "people"),
  // animals & nature: animals, plants, sky and weather
  (0x2600...0x2604, "animals"), (0x2614...0x2614, "animals"), (0x2618...0x2618, "animals"), (0x26A1...0x26A1, "animals"),
  (0x26C4...0x26C8, "animals"), (0x2744...0x2744, "animals"), (0x1F300...0x1F300, "animals"),
  (0x1F308...0x1F308, "animals"), (0x1F30A...0x1F30A, "animals"), (0x1F311...0x1F32C, "animals"),
  (0x1F330...0x1F344, "animals"), (0x1F400...0x1F43F, "animals"), (0x1F490...0x1F490, "animals"),
  (0x1F4AE...0x1F4AE, "animals"), (0x1F525...0x1F525, "animals"), (0x1F54A...0x1F54A, "animals"),
  (0x1F577...0x1F578, "animals"), (0x1F940...0x1F940, "animals"), (0x1F980...0x1F9AE, "animals"),
  (0x1FA90...0x1FA90, "animals"), (0x1FAB0...0x1FABF, "animals"), (0x1FACE...0x1FACF, "animals"),
  (0x1F4A7...0x1F4A7, "animals"), (0x2B50...0x2B50, "animals"), (0x1F31F...0x1F320, "animals"),
  // food & drink
  (0x2615...0x2615, "food"), (0x1F32D...0x1F32F, "food"), (0x1F345...0x1F37F, "food"),
  (0x1F382...0x1F382, "food"), (0x1F3FA...0x1F3FA, "food"), (0x1F52A...0x1F52A, "food"),
  (0x1F942...0x1F944, "food"), (0x1F950...0x1F96F, "food"), (0x1F99E...0x1F99F, "food"),
  (0x1F9C0...0x1F9CB, "food"), (0x1F9C2...0x1F9C2, "food"), (0x1FAD0...0x1FADB, "food"),
  // travel & places: places, buildings, transport, time, sky above the weather
  (0x231A...0x231B, "travel"), (0x23F0...0x23F3, "travel"), (0x2693...0x2693, "travel"), (0x26E9...0x26FD, "travel"),
  (0x2708...0x2708, "travel"), (0x1F301...0x1F307, "travel"), (0x1F309...0x1F309, "travel"),
  (0x1F30B...0x1F310, "travel"), (0x1F3A0...0x1F3A2, "travel"), (0x1F3AA...0x1F3AA, "travel"),
  (0x1F3CD...0x1F3CE, "travel"), (0x1F3D4...0x1F3F0, "travel"), (0x1F488...0x1F488, "travel"),
  (0x1F492...0x1F492, "travel"), (0x1F4BA...0x1F4BA, "travel"), (0x1F550...0x1F567, "travel"),
  (0x1F5FA...0x1F5FE, "travel"), (0x1F680...0x1F6A2, "travel"), (0x1F6A4...0x1F6A9, "travel"),
  (0x1F6B2...0x1F6B2, "travel"), (0x1F6CE...0x1F6D7, "travel"), (0x1F6DC...0x1F6DF, "travel"),
  (0x1F6E3...0x1F6FC, "travel"), (0x1F9ED...0x1F9ED, "travel"), (0x1F9F3...0x1F9F3, "travel"),
  (0x1F9F1...0x1F9F1, "travel"), (0x1FA90...0x1FA90, "travel"), (0x1FA82...0x1FA82, "travel"),
  (0x1F6DE...0x1F6DF, "travel"), (0x1F6E9...0x1F6E9, "travel"),
  // activities: events, sports, games, arts
  (0x265F...0x265F, "activities"), (0x2660...0x2667, "activities"), (0x26BD...0x26BE, "activities"),
  (0x26F3...0x26F3, "activities"), (0x26F8...0x26F8, "activities"), (0x1F004...0x1F004, "activities"),
  (0x1F0CF...0x1F0CF, "activities"), (0x1F380...0x1F38B, "activities"), (0x1F38D...0x1F391, "activities"),
  (0x1F396...0x1F397, "activities"), (0x1F399...0x1F39F, "activities"), (0x1F3A3...0x1F3C6, "activities"),
  (0x1F3C8...0x1F3C9, "activities"), (0x1F3CF...0x1F3D3, "activities"), (0x1F3F8...0x1F3F9, "activities"),
  (0x1F579...0x1F579, "activities"), (0x1F5BC...0x1F5BC, "activities"), (0x1F93A...0x1F93B, "activities"),
  (0x1F93F...0x1F93F, "activities"), (0x1F945...0x1F94F, "activities"), (0x1F9E7...0x1F9E9, "activities"),
  (0x1F9F5...0x1F9F6, "activities"), (0x1F9F8...0x1F9F8, "activities"), (0x1FA80...0x1FA81, "activities"),
  (0x1FA83...0x1FA88, "activities"), (0x1F52E...0x1F52E, "activities"),
  // objects: the rest of the pictographs
  (0x2328...0x2328, "objects"), (0x23CF...0x23CF, "objects"), (0x260E...0x260E, "objects"),
  (0x2692...0x2692, "objects"), (0x2694...0x2697, "objects"), (0x2699...0x2699, "objects"),
  (0x26B0...0x26B1, "objects"), (0x26CF...0x26CF, "objects"), (0x26D1...0x26D1, "objects"),
  (0x26D3...0x26D3, "objects"), (0x2702...0x2702, "objects"), (0x2709...0x2709, "objects"),
  (0x270F...0x270F, "objects"), (0x2712...0x2712, "objects"), (0x1F380...0x1F5FF, "objects"),
  (0x1F6AA...0x1F6AD, "objects"), (0x1F6BD...0x1F6BD, "objects"), (0x1F6BF...0x1F6BF, "objects"),
  (0x1F6C1...0x1F6C5, "objects"), (0x1F6CB...0x1F6CB, "objects"), (0x1F6D0...0x1F6D2, "objects"),
  (0x1F6E0...0x1F6E2, "objects"), (0x1F900...0x1F9FF, "objects"), (0x1FA70...0x1FAFF, "objects"),
  // symbols: everything else, arrows, signs, shapes, letters
  (0x00A9...0x2BFF, "symbols"), (0x3000...0x33FF, "symbols"), (0x1F100...0x1F2FF, "symbols"),
  (0x1F680...0x1F6FF, "symbols"), (0x1F7E0...0x1F7FF, "symbols"),
]

func groupOf(_ value: UInt32) -> String {
  for (range, id) in rules where range.contains(value) { return id }
  return "symbols"
}

func add(_ emoji: String, _ name: String, to id: String) {
  guard let index = groups.firstIndex(where: { $0.id == id }) else { return }
  groups[index].emoji.append((emoji, name))
}

for value in UInt32(0xA9)...UInt32(0x1FAFF) {
  guard let scalar = Unicode.Scalar(value) else { continue }
  let properties = scalar.properties
  let presented = properties.isEmojiPresentation
  guard presented || (properties.isEmoji && value >= 0x2000) else { continue }
  if (0x1F3FB...0x1F3FF).contains(value) || (0x1F1E6...0x1F1FF).contains(value) { continue }
  guard let name = properties.name, !name.hasPrefix("EMOJI COMPONENT") else { continue }
  let text = presented ? String(scalar) : String(scalar) + "\u{FE0F}"
  guard drawnAsOne(text) else { continue }
  add(text, name.lowercased(), to: groupOf(value))
}

let english = Locale(identifier: "en")
for region in Locale.Region.isoRegions.map(\.identifier).sorted() {
  let letters = region.unicodeScalars
  guard letters.count == 2, letters.allSatisfy({ ("A"..."Z").contains($0) }) else { continue }
  let flag = String(String.UnicodeScalarView(letters.map { Unicode.Scalar(0x1F1E6 + $0.value - 0x41)! }))
  guard drawnAsOne(flag), let name = english.localizedString(forRegionCode: region) else { continue }
  add(flag, "flag: \(name.lowercased())", to: "flags")
}

// quoted makes a TypeScript string literal
func quoted(_ text: String) -> String {
  var out = "\""
  for character in text.unicodeScalars {
    switch character {
    case "\"": out += "\\\""
    case "\\": out += "\\\\"
    default: out.unicodeScalars.append(character)
    }
  }
  return out + "\""
}

print("// Generated by web/scripts/emoji.swift on macOS from the Swift standard library's Unicode data (names and emoji")
print("// properties) and the system emoji font: do not edit. The folder-icon picker's emoji, in nine groups.")
print("")
print("export type EmojiGroup = { id: string; name: string; emoji: [string, string][] }; // [the emoji, its name]")
print("")
print("export const EMOJI_GROUPS: EmojiGroup[] = [")
for group in groups {
  let items = group.emoji.map { "[\(quoted($0.0)), \(quoted($0.1))]" }.joined(separator: ", ")
  print("  { id: \(quoted(group.id)), name: \(quoted(group.name)), emoji: [\(items)] },")
}
print("];")
let counts = groups.map { "\($0.id) \($0.emoji.count)" }.joined(separator: ", ")
FileHandle.standardError.write("total \(groups.reduce(0) { $0 + $1.emoji.count }): \(counts)\n".data(using: .utf8)!)
