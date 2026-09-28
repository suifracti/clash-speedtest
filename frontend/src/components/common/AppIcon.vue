<script setup lang="ts">
defineProps<{ name: 'dashboard' | 'monitor' | 'history' | 'subscription' | 'settings' | 'backup' | 'pulse' | 'download' | 'service' }>()
const paths = {
  dashboard: 'M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z',
  monitor: 'M3 12h4l3-7 4 14 3-7h4',
  history: 'M3 5v5h5 M3 10a9 9 0 1 1 2 8 M12 7v5l3 2',
  subscription: 'M4 5h16v14H4z M4 9h16 M8 14h3 M15 14h2',
  settings: 'M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8 M12 2v3 M12 19v3 M2 12h3 M19 12h3 M5 5l2 2 M17 17l2 2 M5 19l2-2 M17 7l2-2',
  backup: 'M4 7h16v14H4z M2 3h20v4H2z M9 12h6',
  pulse: 'M2 12h4l3-7 5 14 3-7h5',
  download: 'M12 3v12 M7 10l5 5 5-5 M4 16v5h16v-5',
  service: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0 M3 12h18 M12 3c5 5 5 13 0 18-5-5-5-13 0-18',
}
</script>
<template><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" class="app-icon"><path :d="paths[name]" /></svg></template>
<style scoped>.app-icon { width:18px; height:18px; flex:none; }</style>
