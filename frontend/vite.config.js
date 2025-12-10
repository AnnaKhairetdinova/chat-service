import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
    plugins: [vue()],
    server: {
        port: 5173,
        host: true,
        proxy: {
            '^/api': {
                target: 'http://127.0.0.1:8080',
                changeOrigin: true,
                secure: false,
            },
            '^/ws': {
                target: 'ws://127.0.0.1:8080',
                ws: true,
                changeOrigin: true,
            },
        },
    },
})
