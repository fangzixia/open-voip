// 本文件负责Vite 构建配置、入口和运行时接口地址注入。
import { resolve } from "node:path";
import { defineConfig, loadEnv } from "vite";

const rootDir = import.meta.dirname;

/** @type {import('vite').UserConfig} */
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, resolve(rootDir), "");
  const apiBase = (env.VITE_API_BASE || "").replace(/\/$/, "");
  const injectSnippet = apiBase
    ? `<script>window.__OPEN_VOIP__=Object.assign(window.__OPEN_VOIP__||{},{apiBase:${JSON.stringify(apiBase)}});</script>`
    : "";

  return {
    root: resolve(rootDir),
    base: "./",
    build: {
      outDir: resolve(rootDir, "dist"),
      emptyOutDir: true,
      rollupOptions: {
        input: {
          app: resolve(rootDir, "index.html"),
          guest: resolve(rootDir, "guest/index.html"),
        },
      },
    },
    server: {
      host: true,
      port: 5173,
      allowedHosts: [".trycloudflare.com"],
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
          const faviconSnippet =
            '<link rel="icon" href="/favicon.ico" sizes="32x32" />' +
            '<link rel="icon" href="/favicon.svg" type="image/svg+xml" />' +
            '<link rel="apple-touch-icon" href="/favicon.svg" />';
          let out = html.replace(
            /<link rel="(?:icon|apple-touch-icon)"[^>]*>\s*/g,
            "",
          );
          out = out.replace("</head>", `${faviconSnippet}${injectSnippet}</head>`);
          return out;
        },
      },
    ],
  };
});
