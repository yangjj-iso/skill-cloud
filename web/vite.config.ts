import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// The Vite dev server proxies /v1, /mcp and /metrics to the Skill Cloud
// server so the UI can call the API without CORS handling. Override the
// target with the VITE_SKILLCLOUD_API env var when developing against a
// remote deployment.
const env = loadEnv("development", process.cwd(), "");
const apiTarget = env.VITE_SKILLCLOUD_API || env.SKILLCLOUD_API || "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
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
