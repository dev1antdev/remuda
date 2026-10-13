import { fileURLToPath, URL } from "node:url";
import { defineConfig } from "vite";
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
});
