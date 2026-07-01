<script setup lang="ts">
import EditorShell from './components/editor-shell.vue';
import { useSiteConfigEditor } from './use-site-config-editor';

const { config, loading, saveConfig, saving } = useSiteConfigEditor();
</script>

<template>
  <EditorShell
    description="只读预览当前 shared/site-config.json 结构，方便确认字段。"
    :loading="loading"
    :saving="saving"
    title="JSON 高级"
    @save="saveConfig"
  >
    <pre v-if="config" class="json-preview">{{
      JSON.stringify(config, null, 2)
    }}</pre>
  </EditorShell>
</template>

<style scoped>
.json-preview {
  max-height: 70vh;
  padding: 16px;
  margin: 0;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  background: hsl(var(--background-deep));
  border-radius: 8px;
}
</style>
