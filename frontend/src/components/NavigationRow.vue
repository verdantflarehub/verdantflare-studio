<script setup lang="ts">
import NavigationIcon from './NavigationIcon.vue'

const props = defineProps<{
  label: string
  icon?: string
  href?: string
  children?: boolean
  expanded?: boolean
  controls?: string
  current?: boolean
  parentCurrent?: boolean
  child?: boolean
  count?: string | number
  badge?: string
}>()
const emit = defineEmits<{ activate: []; toggle: [] }>()
function activate(event: MouseEvent) {
  // Keep ordinary browser link actions (new tab, copy link) available.
  if (props.href && (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0)) return
  event.preventDefault()
  if (props.href) emit('activate')
  else emit('toggle')
}
</script>

<template>
  <div class="menu-row" :class="{ child, current, 'parent-current': parentCurrent }">
    <component :is="href ? 'a' : 'button'" class="row-action" :href="href" :type="href ? undefined : 'button'"
      :title="label" :aria-label="label" :aria-current="current ? 'page' : undefined"
      :aria-expanded="children && !href ? !!expanded : undefined" :aria-controls="children && !href ? controls : undefined" @click="activate">
      <span v-if="icon" class="icon-slot"><NavigationIcon :name="icon" /></span>
      <span class="label">{{ label }}</span>
      <span v-if="count !== undefined" class="count">{{ count }}</span>
      <span v-if="badge" class="badge">{{ badge }}</span>
      <NavigationIcon v-if="children && !href" name="chevron" />
    </component>
    <button v-if="children && href" type="button" class="row-toggle" :aria-label="(expanded ? '收起' : '展开') + label"
      :aria-expanded="!!expanded" :aria-controls="controls" @click="emit('toggle')"><NavigationIcon name="chevron" /></button>
  </div>
</template>

<style scoped>
.menu-row { display:flex; align-items:stretch; width:100%; min-width:0; border-radius:7px; color:var(--muted); transition:background var(--vf-dur-fast,140ms) var(--vf-ease-out),color var(--vf-dur-fast,140ms) var(--vf-ease-out); }
.menu-row:hover,.menu-row.current { background:var(--shell-button); color:var(--ink); }
.menu-row.current,.menu-row.parent-current { color:var(--ink); font-weight:600; }
.row-action { display:flex; flex:1; min-width:0; align-items:center; gap:6px; min-height:40px; padding:9px 12px 9px 6px; border-radius:inherit; text-align:left; line-height:22px; color:inherit; background:transparent; font-weight:inherit; }
.icon-slot { flex:0 0 30px; display:grid; place-items:center; }
.label { flex:1; min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.row-action>.chevron { flex:0 0 14px; }
.row-toggle { flex:0 0 38px; padding:0 12px 0 0; display:grid; place-items:center end; color:inherit; background:transparent; border-radius:inherit; }
.row-action:hover,.row-toggle:hover { background:transparent; }
.row-action:focus-visible,.row-toggle:focus-visible { outline:2px solid var(--accent); outline-offset:-2px; }
.chevron { transition:transform var(--vf-dur-Spring,340ms) var(--vf-ease-spring); }
[aria-expanded=true]>.chevron { transform:rotate(180deg); }
.current:not(.child) .icon-slot { color:var(--accent); }
.count,.badge { color:var(--muted); font-size:11px; font-weight:400; font-variant-numeric:tabular-nums; }
.child { position:relative; }
.child .row-action { min-height:35px; padding:6px 12px 6px 42px; font-size:13px; }
.child.current:before { content:""; position:absolute; width:4px; height:4px; left:24px; top:calc(50% - 2px); border-radius:50%; background:var(--accent); pointer-events:none; }
@media(max-width:700px) {
  .row-action { justify-content:center; padding:10px 8px; }
  .icon-slot { flex-basis:19px; }
  .menu-row:has(.row-toggle) .row-action { padding:10px 0; }
  .row-toggle { flex-basis:20px; }
  .label,.count,.badge,.row-action>.chevron { display:none; }
  .child .row-action { padding:8px 3px; min-height:34px; font-size:10px; line-height:15px; text-align:center; }
  .child .label { display:block; white-space:normal; }
  .child.current:before { display:none; }
}
@media(prefers-reduced-motion:reduce) { * { transition:none!important; } }
</style>
