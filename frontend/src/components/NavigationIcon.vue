<script setup lang="ts">
defineProps<{ name: string }>()
const paths: Record<string, string> = {
  folder: 'M3 7h7l2 2h9v10H3zM3 7V4h8l2 2h8v3',
  globe: 'M3 12h18M12 3c-3 3-3 15 0 18M12 3c3 3 3 15 0 18M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0',
  network: 'M13 2L3 14h9l-1 8 10-12h-9l1-8z',
  resources: 'M3 3h18v7H3zM3 14h18v7H3zM6 6h1M6 17h1',
  server: 'M3 3h18v7H3zM3 14h18v7H3zM6 6h1M6 17h1',
  app: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
  chevron: 'm7 10 5 5 5-5',
  theme: 'M12 3a9 9 0 1 0 9 9 7 7 0 0 1-9-9Z',
  help: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0M9.5 9a2.5 2.5 0 1 1 4 2c-1.5 1-1.5 1-1.5 3m0 3h.01',
}
</script>
<template><svg class="icon" :class="{ chevron: name === 'chevron' }" viewBox="0 0 24 24" aria-hidden="true"><path :d="paths[name] || paths.app" /></svg></template>
<style scoped>
.icon { width:19px; height:19px; flex:none; stroke:currentColor; stroke-width:1.6; fill:none; stroke-linecap:round; stroke-linejoin:round; }
.chevron { width:14px; height:14px; transition:transform .16s; }
</style>
