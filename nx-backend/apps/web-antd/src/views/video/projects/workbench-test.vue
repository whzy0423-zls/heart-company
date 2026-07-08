<template>
  <div style="padding: 20px;">
    <h1>工作台测试页面</h1>
    <p>项目 ID: {{ projectId }}</p>
    <p>如果你能看到这个页面，说明路由是正常的</p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const projectId = computed(() => route.params.id as string)
</script>
