<script setup>
import { computed } from 'vue'
const props = defineProps({ name: { type: String, default: 'arrow' }, size: { type: [Number, String], default: 20 }, color: { type: String, default: '#A55C3B' } })
const paths = {
  arrow: '<path d="M5 12h14m-5-5 5 5-5 5"/>',
  chevron: '<path d="m9 5 7 7-7 7"/>',
  play: '<path d="m9 5 11 7-11 7z"/>',
  video: '<rect x="3" y="5" width="18" height="14" rx="3"/><path d="m10 9 5 3-5 3z"/>',
  book: '<path d="M12 5v15M3 4c4-1 6 0 9 2 3-2 5-3 9-2v14c-4-1-6 0-9 2-3-2-5-3-9-2z"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21v-2a8 8 0 0 1 16 0v2"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="3"/><path d="M7 3v4m10-4v4M3 11h18m-13 5h3"/>',
  clock: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
  heart: '<path d="M20 5a5 5 0 0 0-8 1 5 5 0 0 0-8-1c-5 5 3 11 8 15 5-4 13-10 8-15z"/>',
  relation: '<circle cx="8" cy="12" r="6"/><circle cx="16" cy="12" r="6"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  home: '<path d="m3 10 9-7 9 7M5 9v12h14V9M9 21v-7h6v7"/>',
  grid: '<rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/>',
  spark: '<path d="m12 2 2.5 7.5L22 12l-7.5 2.5L12 22l-2.5-7.5L2 12l7.5-2.5z"/>',
  message: '<path d="M21 11a9 9 0 0 1-13 8l-5 2 1-5A9 9 0 1 1 21 11z"/><path d="M8 10h8m-8 4h5"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 5 5"/>',
}
const source = computed(() => 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="${/^#[0-9a-f]{3,8}$/i.test(props.color) ? props.color : '#A55C3B'}" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">${paths[props.name] || paths.arrow}</svg>`))
</script>

<template><image class="studio-icon" :src="source" mode="aspectFit" :style="{ width: `${size}px`, height: `${size}px` }" aria-hidden="true" /></template>
<style scoped>.studio-icon { display: inline-block; flex-shrink: 0; vertical-align: middle; }</style>
