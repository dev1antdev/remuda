import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { canWake, isMAC, wokeWords } from "./wake";

describe("wake", () => {
  afterEach(() => setLang("en"));
  it("offers Wake up only for an away machine with an address", () => {
    expect(canWake({ state: "error", mac: "aa:bb:cc:dd:ee:ff" })).toBe(true);
    expect(canWake({ state: "connecting", mac: "aa:bb:cc:dd:ee:ff" })).toBe(true);
    expect(canWake({ state: "connected", mac: "aa:bb:cc:dd:ee:ff" })).toBe(false);
    expect(canWake({ state: "error" })).toBe(false);
  });
  it("reads the three spellings of an address and refuses the rest", () => {
    expect(isMAC("AA:BB:CC:DD:EE:FF")).toBe(true);
    expect(isMAC(" aa-bb-cc-dd-ee-ff ")).toBe(true);
    expect(isMAC("aabb.ccdd.eeff")).toBe(true);
    expect(isMAC("aa:bb:cc:dd:ee")).toBe(false);
    expect(isMAC("hello")).toBe(false);
    expect(isMAC("")).toBe(false);
  });
  it("says what the packet did", () => {
    expect(wokeWords("GOG", 2)).toBe("Woke GOG — reconnecting");
    expect(wokeWords("GOG", 0)).toBe("Nothing could carry the packet to GOG");
  });
  it("says it in the page's language, the machine's name filled in after the lookup", () => {
    setLang("ru");
    expect(wokeWords("GOG", 2)).toBe("Разбудили GOG, переподключаемся");
    expect(wokeWords("GOG", 0)).toBe("Пакет для GOG отправить было некуда");
  });
});
