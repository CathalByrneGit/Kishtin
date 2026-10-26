import { svelte } from "@sveltejs/vite-plugin-svelte";
import { defineConfig } from "vite";

// The kit-ui source lives in vendor/kit-ui (a git submodule) and is linked
// into node_modules. Its imports resolve from this app's node_modules, so
// there is one copy of Svelte and the icon set.
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    dedupe: ["svelte", "@lucide/svelte", "dompurify", "marked", "shiki", "@videojs/html"],
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
});
