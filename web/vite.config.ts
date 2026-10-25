import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  // the build's version, so a page can tell it is older than the server
  define: { __REMUDA_VERSION__: JSON.stringify(process.env.REMUDA_VERSION ?? "dev") },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:7317",
      "/ws": { target: "ws://127.0.0.1:7317", ws: true },
    },
  },
  build: { outDir: "dist" },
  // the pure modules have tests beside them; nothing needs a DOM; the stylesheet is read as text by lib/theme.test.ts
  test: { include: ["src/**/*.test.ts"], environment: "node", css: { include: /styles\.css/ } },
});
