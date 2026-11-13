import { describe, expect, it } from "vitest";
import { absolutePath, basename, dirname, joinPath, pathCrumbs, pathParts, separatorOf, underFolder, windowsPath } from "./pathname";

describe("pathname", () => {
  it("tells a path that starts at a root", () => {
    for (const p of ["/a", "C:\\Users\\me", "c:/work", "\\\\nas\\share"]) expect(absolutePath(p)).toBe(true);
    for (const p of ["", "a/b", "~/x", "C:", "src\\App.cs", "C:relative"]) expect(absolutePath(p)).toBe(false);
  });

  it("tells a Windows path from a POSIX one", () => {
    for (const p of ["C:\\Users\\me\\app", "c:/work", "D:", "\\\\nas\\share\\x", "src\\App.xaml.cs"]) expect(windowsPath(p)).toBe(true);
    for (const p of ["/a/b", "~/x", "a/b\\c", "plain", "/tmp/C:\\odd"]) expect(windowsPath(p)).toBe(false);
  });
  it("basename and dirname take either separator on a Windows path", () => {
    expect(basename("C:\\Users\\me\\app")).toBe("app");
    expect(basename("C:\\Users\\me\\app\\")).toBe("app");
    expect(basename("C:\\")).toBe("C:");
    expect(basename("C:/Users/me/Form1.cs")).toBe("Form1.cs");
    expect(dirname("C:\\Users\\me\\Form1.cs")).toBe("C:\\Users\\me\\");
    expect(dirname("src\\App.xaml.cs")).toBe("src\\");
  });
  it("keeps the POSIX cases, a backslash there a character of the name", () => {
    expect(basename("/a/b/c.ts")).toBe("c.ts");
    expect(basename("/a/b/")).toBe("b");
    expect(basename("plain")).toBe("plain");
    expect(dirname("/a/b/c.ts")).toBe("/a/b/");
    expect(dirname("c.ts")).toBe("");
    expect(basename("/a/odd\\name")).toBe("odd\\name");
    expect(dirname("/a/odd\\name")).toBe("/a/");
  });
  it("joins with the folder's own separator", () => {
    expect(joinPath("C:\\Users\\me", "app")).toBe("C:\\Users\\me\\app");
    expect(joinPath("C:\\", "Users")).toBe("C:\\Users");
    expect(joinPath("/home/me", "app")).toBe("/home/me/app");
    expect(joinPath("/", "tmp")).toBe("/tmp");
    expect(joinPath("C:/work", "x")).toBe("C:/work/x");
    expect(separatorOf("C:\\a")).toBe("\\");
  });
  it("parts a path by its own separators", () => {
    expect(pathParts("/Users/me/.herdr/worktrees/app/fix")).toEqual(["Users", "me", ".herdr", "worktrees", "app", "fix"]);
    expect(pathParts("C:\\Users\\me\\.herdr\\worktrees\\app\\fix\\")).toEqual(["C:", "Users", "me", ".herdr", "worktrees", "app", "fix"]);
  });
  it("gives the path under a folder, case-blind on Windows", () => {
    expect(underFolder("/r/web/src/a.ts", "/r")).toBe("web/src/a.ts");
    expect(underFolder("/r/web/src/a.ts", "/r/")).toBe("web/src/a.ts");
    expect(underFolder("/rx/a.ts", "/r")).toBe(null);
    expect(underFolder("/R/a.ts", "/r")).toBe(null);
    expect(underFolder("C:\\Users\\me\\app\\src\\Form1.cs", "C:\\Users\\me\\app")).toBe("src\\Form1.cs");
    expect(underFolder("c:\\users\\me\\app\\Form1.cs", "C:\\Users\\me\\app\\")).toBe("Form1.cs");
    expect(underFolder("C:/Users/me/app/Form1.cs", "C:\\Users\\me\\app")).toBe("Form1.cs");
    expect(underFolder("C:\\Users\\me\\apps\\x", "C:\\Users\\me\\app")).toBe(null);
  });
  it("lays the crumbs out from the root", () => {
    expect(pathCrumbs("/Users/me")).toEqual([
      { label: "/", path: "/" },
      { label: "Users", path: "/Users" },
      { label: "me", path: "/Users/me" },
    ]);
    expect(pathCrumbs("/")).toEqual([{ label: "/", path: "/" }]);
    expect(pathCrumbs("C:\\Users\\me")).toEqual([
      { label: "C:\\", path: "C:\\" },
      { label: "Users", path: "C:\\Users" },
      { label: "me", path: "C:\\Users\\me" },
    ]);
    expect(pathCrumbs("C:\\")).toEqual([{ label: "C:\\", path: "C:\\" }]);
    expect(pathCrumbs("C:/work/app").map((c) => c.path)).toEqual(["C:/", "C:/work", "C:/work/app"]);
    expect(pathCrumbs("\\\\nas\\share\\docs").map((c) => c.label)).toEqual(["\\\\nas\\share\\", "docs"]);
    expect(pathCrumbs("")).toEqual([]);
  });
});
