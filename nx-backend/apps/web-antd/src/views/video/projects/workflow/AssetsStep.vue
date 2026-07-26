<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';

import {
  Button as AButton,
  Input as AInput,
  Modal,
  Modal as AModal,
  Textarea as ATextarea,
  message,
} from 'ant-design-vue';

import {
  createCharacterApi,
  createSceneApi,
  deleteCharacterApi,
  deleteSceneApi,
  listCharactersApi,
  listScenesApi,
  updateCharacterApi,
  updateSceneApi,
  type Character,
  type Scene,
} from '#/api/core/videoproject';

const props = defineProps<{ projectId: string }>();
const emit = defineEmits<{ changed: []; dirty: [value: boolean] }>();
const characters = ref<Character[]>([]);
const scenes = ref<Scene[]>([]);
const loading = ref(false);
const modalOpen = ref(false);
const editorType = ref<'character' | 'scene'>('character');
const editingId = ref('');
const form = reactive({ name: '', description: '', referenceImageUrl: '', referenceVideoUrl: '' });
const itemError = ref('');

const missingCharacterImages = computed(() => characters.value.filter((item) => !item.referenceImageUrl).length);
const missingSceneImages = computed(() => scenes.value.filter((item) => !item.referenceImageUrl).length);

async function load() {
  loading.value = true;
  try {
    [characters.value, scenes.value] = await Promise.all([
      listCharactersApi(props.projectId),
      listScenesApi(props.projectId),
    ]);
  } finally {
    loading.value = false;
  }
}

function openEditor(type: 'character' | 'scene', item?: Character | Scene) {
  editorType.value = type;
  editingId.value = item?.id || '';
  Object.assign(form, {
    description: item?.description || '', name: item?.name || '',
    referenceImageUrl: item?.referenceImageUrl || '',
    referenceVideoUrl: type === 'scene' ? (item as Scene | undefined)?.referenceVideoUrl || '' : '',
  });
  itemError.value = '';
  modalOpen.value = true;
}

async function saveItem() {
  if (!form.name.trim()) {
    itemError.value = '请填写名称';
    return;
  }
  emit('dirty', true);
  if (editorType.value === 'character') {
    const payload = { description: form.description, name: form.name, referenceImageUrl: form.referenceImageUrl };
    if (editingId.value) await updateCharacterApi(editingId.value, payload);
    else await createCharacterApi(props.projectId, payload);
  } else {
    const payload = { description: form.description, name: form.name, referenceImageUrl: form.referenceImageUrl, referenceVideoUrl: form.referenceVideoUrl };
    if (editingId.value) await updateSceneApi(editingId.value, payload);
    else await createSceneApi(props.projectId, payload);
  }
  modalOpen.value = false;
  emit('dirty', false);
  emit('changed');
  await load();
}

function removeItem(type: 'character' | 'scene', id: string) {
  Modal.confirm({
    title: `删除${type === 'character' ? '角色' : '场景'}？`,
    content: '引用它的分镜会标记为需要重新生成。',
    async onOk() {
      if (type === 'character') await deleteCharacterApi(id);
      else await deleteSceneApi(id);
      emit('changed');
      await load();
    },
  });
}

function openAssetLibrary() {
  message.info('资产库已在新标签打开');
  window.open('/video/assets', '_blank', 'noopener,noreferrer');
}

onMounted(load);
defineExpose({ save: async () => undefined });
</script>

<template>
  <div class="assets-step" :aria-busy="loading">
    <div class="asset-toolbar">
      <div><strong>角色 {{ characters.length }}</strong><span>缺少参考图 {{ missingCharacterImages }}</span></div>
      <div><strong>场景 {{ scenes.length }}</strong><span>缺少参考图 {{ missingSceneImages }}</span></div>
      <a-button @click="openAssetLibrary">打开资产库</a-button>
    </div>
    <section>
      <div class="section-head"><h3>角色</h3><a-button @click="openEditor('character')">添加角色</a-button></div>
      <div class="asset-list">
        <article v-for="item in characters" :key="item.id">
          <img v-if="item.referenceImageUrl" :src="item.referenceImageUrl" :alt="`${item.name}参考图`" />
          <div v-else class="missing-image">缺少参考图</div>
          <strong>{{ item.name }}</strong><span>{{ item.description }}</span>
          <div><a-button @click="openEditor('character', item)">编辑</a-button><a-button danger @click="removeItem('character', item.id)">删除</a-button></div>
        </article>
      </div>
    </section>
    <section>
      <div class="section-head"><h3>场景</h3><a-button @click="openEditor('scene')">添加场景</a-button></div>
      <div class="asset-list">
        <article v-for="item in scenes" :key="item.id">
          <img v-if="item.referenceImageUrl" :src="item.referenceImageUrl" :alt="`${item.name}参考图`" />
          <div v-else class="missing-image">缺少参考图</div>
          <strong>{{ item.name }}</strong><span>{{ item.description }}</span>
          <div><a-button @click="openEditor('scene', item)">编辑</a-button><a-button danger @click="removeItem('scene', item.id)">删除</a-button></div>
        </article>
      </div>
    </section>
    <p v-if="itemError" role="alert" class="item-error">{{ itemError }} <a-button type="link" @click="saveItem">重试</a-button></p>
    <a-modal v-model:open="modalOpen" :title="editingId ? '编辑' : '新增'" @ok="saveItem">
      <div class="editor-form"><label>名称<a-input v-model:value="form.name" /></label><label>描述<a-textarea v-model:value="form.description" /></label><label>参考图 URL<a-input v-model:value="form.referenceImageUrl" /></label><label v-if="editorType === 'scene'">参考视频 URL<a-input v-model:value="form.referenceVideoUrl" /></label></div>
    </a-modal>
  </div>
</template>

<style scoped>
.assets-step { display: grid; gap: 28px; }.asset-toolbar,.section-head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; }.asset-toolbar > div { display: grid; }.asset-toolbar span { color:#64748b; }.asset-list { display:grid; grid-template-columns:repeat(auto-fill,minmax(210px,1fr)); gap:12px; }article { min-width:0; padding:12px; display:grid; gap:8px; border:1px solid #dbe3ee; border-radius:8px; background:#fff; }article img,.missing-image { width:100%; aspect-ratio:16/10; object-fit:cover; display:flex; align-items:center; justify-content:center; color:#64748b; background:#f1f5f9; }article span { color:#64748b; overflow-wrap:anywhere; }.editor-form,label { display:grid; gap:8px; }.editor-form { gap:16px; }.item-error { color:#b91c1c; }:deep(.ant-btn),:deep(.ant-input) { min-height:44px; }
</style>
