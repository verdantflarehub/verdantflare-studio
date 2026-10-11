<script setup lang="ts">
import { computed } from 'vue'
import { dramaText } from '../../platform/drama-presentation'
import type { ProjectContent } from '../../platform/project-content'
const props = defineProps<{ text?: string | null; drama?: ProjectContent['ai_short_drama']; hideProductionNotes?: boolean }>()
type Line = { heading?: boolean; cells: { label?: string; parts: string[] }[] }
// Render authored Markdown as escaped Vue text; never execute embedded HTML.
const lines = computed(() => {
  let headers: string[] = []
  const result: Line[] = []
  const source = dramaText(props.text, props.drama).split('\n')
  for (const [index, raw] of source.entries()) {
    const line = raw.trim()
    if (!line || /^<a\s+(?:id|name)=["'][^"']+["']\s*>\s*<\/a>$/.test(line)) continue
    if (props.hideProductionNotes && /^\*\*资产：/.test(line)) continue
    if (line.startsWith('|')) {
      const cells = line.replace(/^\||\|$/g, '').split('|').map(s => s.trim())
      if (cells.every(c => /^[-: ]+$/.test(c))) continue
      if (!headers.length && /^\s*\|[\s|:\-]+\|\s*$/.test(source[index + 1] || '')) { headers = cells; continue }
      result.push({ cells: cells.map((c, i) => ({ label: headers.length ? (headers[i] || '说明').replace(/\*\*/g, '') : undefined, parts: c.split(/(\*\*[^*]+\*\*)/g) })) })
    } else {
      headers = []
      result.push({ heading: /^#{1,6}\s/.test(line), cells: [{ parts: line.replace(/^#{1,6}\s+/, '').replace(/^[->]\s+/, '').split(/(\*\*[^*]+\*\*)/g) }] })
    }
  }
  return result
})
</script>
<template><div class="drama-prose"><div v-for="(line, i) in lines" :key="i" :class="{ 'text-record': line.cells.some(c => c.label) }"><component :is="line.heading ? 'h3' : 'p'" v-for="(cell, k) in line.cells" :key="k"><strong v-if="cell.label">{{ cell.label }}：</strong><template v-for="(part, j) in cell.parts" :key="j"><strong v-if="part.startsWith('**') && part.endsWith('**')">{{ part.slice(2, -2) }}</strong><template v-else>{{ part }}</template></template></component></div></div></template>
<style scoped>
.drama-prose { font-size:14px; line-height:1.95; overflow-wrap:anywhere; }p { margin:0 0 8px; white-space:pre-wrap; }h3 { font-size:17px; font-weight:600; margin:22px 0 12px; }strong { font-weight:600; }.text-record { padding:14px 0; border-bottom:1px solid var(--line); }
</style>
