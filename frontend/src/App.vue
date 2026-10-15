<script setup lang="ts">
import HostSidebar from './components/HostSidebar.vue'
import HostLoginDialog from './components/HostLoginDialog.vue'
</script>

<template>
  <div class="studio-layout">
    <HostSidebar />
    <RouterView />
    <HostLoginDialog />
    <div id="toast" class="toast" role="status" aria-live="polite"></div>
  </div>
</template>

<style>
.studio-layout { display:grid; grid-template-columns:var(--sidebar-width) minmax(0,1fr); min-height:100dvh; }
.studio-layout > .main { margin-left:0; }
@media(max-width:700px) { .studio-layout { --sidebar-width:64px; } }
</style>
