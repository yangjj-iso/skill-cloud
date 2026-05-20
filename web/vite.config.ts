import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// The Vite dev server proxies /v1, /mcp and /metrics to the Skill Cloud
// server so the UI can call the API without CORS handling. Override the
// target with the VITE_SKILLCLOUD_API env var when developing against a
// remote deployment.
const env = loadEnv("development", process.cwd(), "");
const apiTarget = env.VITE_SKILLCLOUD_API || env.SKILLCLOUD_API || "http://localhost:8080";

// The dev server's Host-header allowlist. Defaults to true (any host) so
// that LAN access, codespaces, and tunneled previews (ngrok / cloudflare /
// Devin's *.devinapps.com tunnels) all work out of the box. Override with a
// comma-separated list via VITE_ALLOWED_HOSTS when you want tighter
// rejection. Vite's `allowedHosts: true` disables host-header checks
// entirely for the dev server only (build artefacts are unaffected).
const allowedHostsEnv = env.VITE_ALLOWED_HOSTS;
const allowedHosts: true | string[] = allowedHostsEnv
  ? allowedHostsEnv.split(",").map((s) => s.trim()).filter(Boolean)
  : true;

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    port: 5173,
    allowedHosts,
    proxy: {
      "/v1": apiTarget,
      "/mcp": apiTarget,
      "/metrics": apiTarget,
      "/healthz": apiTarget,
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
  },
});
