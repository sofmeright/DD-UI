// ui/vite.config.ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) }
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    chunkSizeWarningLimit: 700,
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-react': ['react', 'react-dom', 'react-router-dom'],
          'vendor-editor': ['@monaco-editor/react', 'monaco-editor'],
          'vendor-ui': ['lucide-react', 'clsx', 'tailwind-merge'],
          'vendor-terminal': ['@xterm/xterm', '@xterm/addon-fit']
        }
      }
    }
  }
});
