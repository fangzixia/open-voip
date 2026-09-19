import { resolve } from "node:path";
import { defineConfig, loadEnv } from "vite";

/** @type {import('vite').UserConfig} */
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, resolve(__dirname), "");
  const apiBase = (env.VITE_API_BASE || "").replace(/\/$/, "");
  const injectSnippet = apiBase
    ? `<script>window.__OPEN_VOIP__=Object.assign(window.__OPEN_VOIP__||{},{apiBase:${JSON.stringify(apiBase)}});</script>`
    : "";

  return {
    root: resolve(__dirname),
    base: "./",
    build: {
      outDir: resolve(__dirname, "dist"),
      emptyOutDir: true,
      rollupOptions: {
        input: {
          agent: resolve(__dirname, "agent/index.html"),
          guest: resolve(__dirname, "guest/index.html"),
          admin: resolve(__dirname, "admin/index.html"),
        },
      },
    },
    server: {
      port: 5173,
      proxy: apiBase
        ? {}
        : {
            "/api": { target: "http://127.0.0.1:8080", changeOrigin: true, ws: true },
            "/health": { target: "http://127.0.0.1:8080", changeOrigin: true },
          },
    },
    plugins: [
      {
        name: "inject-open-voip-api-base",
        transformIndexHtml(html) {
          if (!injectSnippet) return html;
          return html.replace("</head>", `${injectSnippet}</head>`);
        },
      },
    ],
  };
});
