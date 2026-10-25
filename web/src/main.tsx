import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "@/app/App";
import { startTheme } from "@/app/theme";
import "./styles.css";

startTheme();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// Plain-http LAN addresses have no service worker; the app just runs uncached there.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    void navigator.serviceWorker.register("/sw.js");
  });
}
