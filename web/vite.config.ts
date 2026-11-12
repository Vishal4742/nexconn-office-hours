import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The Go server owns /api, /auth and the public FAQ pages. In dev Vite proxies them to :8080,
// so the browser sees one origin, the same as production (see vercel.json).
const goServer = "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true, // APP_URL and the GitHub OAuth callback are tied to this port
    proxy: {
      "/api": goServer,
      "/auth": goServer,
      "^/p/[^/]+/[^/]+/faq": goServer,
    },
  },
});
