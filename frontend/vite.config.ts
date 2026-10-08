import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({base:'./',plugins:[vue()],server:{proxy:{'/api':'http://127.0.0.1:8000','/apps/video':'http://127.0.0.1:8000','/studio/api':'http://127.0.0.1:8000','/mcp':'http://127.0.0.1:8000','/v2/artifacts':'http://127.0.0.1:8000'}}})
