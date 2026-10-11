<script setup lang="ts">
import { computed } from 'vue'
import { dramaText } from '../../platform/drama-presentation'
import type { ProjectContent } from '../../platform/project-content'
const props = defineProps<{ text?: string | null; drama?: ProjectContent['ai_short_drama'] }>()
// Vue text nodes deliberately escape HTML; authored text never becomes executable markup.
const lines = computed(() => dramaText(props.text, props.drama).split('\n').filter(line => !/^\s*<a\s+(?:id|name)=["'][^"']+["']\s*>\s*<\/a>\s*$/.test(line)).map(line => ({
  heading: /^#{1,6}\s/.test(line),
  parts: line.replace(/^#{1,6}\s+/, '').split(/(\*\*[^*]+\*\*)/g),
})))
</script>
<template><div class="drama-prose"><p v-for="(line, i) in lines" :key="i" :class="{ 'prose-heading': line.heading, blank: !line.parts.join('').trim() }"><template v-for="(part, j) in line.parts" :key="j"><strong v-if="part.startsWith('**') && part.endsWith('**')">{{ part.slice(2, -2) }}</strong><template v-else>{{ part }}</template></template></p></div></template>
<style scoped>
.drama-prose { font-size: 14px; line-height: 1.95; overflow-wrap: anywhere; }p { margin: 0 0 8px; white-space: pre-wrap; }p.blank { min-height: 10px; margin: 0; }.prose-heading { font-size: 17px; font-weight: 600; margin: 24px 0 12px; }strong { font-weight: 600; }
</style>
