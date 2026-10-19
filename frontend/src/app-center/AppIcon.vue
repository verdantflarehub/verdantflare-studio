<script setup lang="ts">
import { computed, useId } from 'vue'
import { vfLogo, brandPaths } from '../pages/brands'
import higgsfieldLogo from '../assets/higgsfield.svg'
import falLogo from '../assets/fal.png'
import blenderLogo from '../assets/blender.svg'
import comfyuiLogo from '../assets/comfyui.svg'
import ChannelBrandIcon from './ChannelBrandIcon.vue'
const props = defineProps<{ iconRef: string | null }>()
const gradId = useId()
const paths: Record<string, string> = { 'brand:minimax': brandPaths.minimax, 'brand:github': brandPaths.github }
const path = computed(() => props.iconRef && Object.hasOwn(paths, props.iconRef) ? paths[props.iconRef] : null)
</script>

<template>
  <span
    class="app-icon appcenter-icon"
    :class="{
      'is-github': iconRef === 'brand:github',
      'is-mono': ['brand:github', 'brand:chatgpt', 'brand:grok'].includes(iconRef || '')
    }"
    aria-hidden="true"
  >
    <img v-if="iconRef === 'brand:vf'" :src="vfLogo" alt="" />
    <img v-else-if="iconRef === 'brand:higgsfield'" :src="higgsfieldLogo" alt="" />
    <img v-else-if="iconRef === 'brand:fal'" :src="falLogo" alt="" />
    <img v-else-if="iconRef === 'brand:blender'" :src="blenderLogo" alt="" />
    <img v-else-if="iconRef === 'brand:comfyui'" :src="comfyuiLogo" alt="" />
    <ChannelBrandIcon v-else-if="['brand:google', 'brand:chatgpt', 'brand:grok'].includes(iconRef || '')" :brand="iconRef || ''" />
    <svg v-else-if="iconRef === 'brand:minimax' && path" viewBox="0 0 24 24">
      <defs>
        <linearGradient :id="gradId" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#E2167E" />
          <stop offset="100%" stop-color="#FE603C" />
        </linearGradient>
      </defs>
      <path :d="path" :fill="`url(#${gradId})`" />
    </svg>
    <svg v-else-if="path" viewBox="0 0 24 24" fill="currentColor"><path :d="path" /></svg>
    <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></svg>
  </span>
</template>

<style scoped>
.appcenter-icon { color: var(--accent); background: var(--panel); flex-shrink: 0; }
.appcenter-icon.is-github,
.appcenter-icon.is-mono { color: var(--ink); }
.appcenter-icon svg, .appcenter-icon img { width: 26px; height: 26px; object-fit: contain; }
</style>
