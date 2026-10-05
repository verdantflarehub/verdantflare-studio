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
