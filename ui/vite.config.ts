import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/admin": "http://localhost:8080",
      "/setup": "http://localhost:8080",
      "/v1": "http://localhost:8080",
    },
  },
});
