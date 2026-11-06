import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "./i18n";
import { stallWords, toastDue } from "./perf";

afterEach(() => setLang("en"));

describe("stallWords", () => {
  it("says how long and where", () => {
    expect(stallWords({ ms: 4213, where: "on the Wall (13 tiles)", at: 0 })).toBe("The page stalled 4.2 s on the Wall (13 tiles)");
    expect(stallWords({ ms: 250, where: "in Agents, terminal lens, local w12:p1", at: 0 })).toBe("The page stalled 0.3 s in Agents, terminal lens, local w12:p1");
    expect(stallWords({ ms: 51_400, where: "", at: 0 })).toBe("The page stalled 51 s");
  });
  it("says the seconds in the page's language", () => {
    setLang("ru");
    expect(stallWords({ ms: 4213, where: "на Стене (13 плиток)", at: 0 })).toBe("Страница зависла на 4,2 с на Стене (13 плиток)");
    expect(stallWords({ ms: 51_400, where: "", at: 0 })).toBe("Страница зависла на 51 с");
  });
});

describe("toastDue", () => {
  it("toasts a stall the person felt, a minute apart at most", () => {
    expect(toastDue({ ms: 2000, where: "", at: 100_000 }, 0)).toBe(true);
    expect(toastDue({ ms: 1999, where: "", at: 100_000 }, 0)).toBe(false);
    expect(toastDue({ ms: 5000, where: "", at: 100_000 }, 50_000)).toBe(false);
    expect(toastDue({ ms: 5000, where: "", at: 100_000 }, 40_000)).toBe(true);
  });
});
