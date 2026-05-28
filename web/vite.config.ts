import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  build: {
    outDir: '../internal/httpx/web_dist',
    emptyOutDir: true
  },
  server: {
    port: 5173,
    proxy: {
      '/v1': 'http://127.0.0.1:18777',
      '/health': 'http://127.0.0.1:18777'
    }
  }
});
