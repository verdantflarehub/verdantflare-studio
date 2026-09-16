import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
export default defineConfig({base:'./',plugins:[vue()],server:{proxy:{'/studio/api':'http://127.0.0.1:8000'}}})
