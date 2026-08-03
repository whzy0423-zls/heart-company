import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

const webDir = dirname(fileURLToPath(import.meta.url));
export default defineConfig({
    base: "/infinite-canvas/",
    plugins: [react()],
    resolve: {
        alias: {
            "@": resolve(webDir, "src"),
        },
    },
    server: {
        host: "0.0.0.0",
        port: 3100,
    },
    build: {
        outDir: "../web-antd/public/infinite-canvas",
        emptyOutDir: true,
    },
});
