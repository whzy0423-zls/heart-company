<script lang="ts">
import type {
  VideoAsset as PickerVideoAsset,
  VideoAssetType as PickerVideoAssetType,
} from '#/api';

const TYPE_META: Record<
  PickerVideoAssetType,
  { color: string; label: string }
> = {
  scene: { color: 'blue', label: '场景' },
  character: { color: 'green', label: '人物' },
  prop: { color: 'gold', label: '物品' },
  outfit: { color: 'magenta', label: '服装' },
  style: { color: 'cyan', label: '风格' },
  audio: { color: 'orange', label: '音频' },
  video: { color: 'purple', label: '视频' },
};

export const ASSET_PICKER_TYPES = Object.keys(
  TYPE_META,
) as PickerVideoAssetType[];

export function getAllowedAssetTypes(allowTypes?: PickerVideoAssetType[]) {
  return allowTypes && allowTypes.length > 0 ? allowTypes : ASSET_PICKER_TYPES;
}

export function getInitialPickerType(allowTypes?: PickerVideoAssetType[]) {
  return allowTypes && allowTypes.length > 0 ? allowTypes[0] || '' : '';
}

export function normalizePickerQueryType(
  type: '' | PickerVideoAssetType,
  allowTypes?: PickerVideoAssetType[],
) {
  if (!type) return '';
  const allowedTypes = getAllowedAssetTypes(allowTypes);
  return allowedTypes.includes(type) ? type : getInitialPickerType(allowTypes);
}

export function filterAllowedAssets(
  items: PickerVideoAsset[],
  allowTypes?: PickerVideoAssetType[],
) {
  const allowedTypes = getAllowedAssetTypes(allowTypes);
  return items.filter((item) => allowedTypes.includes(item.type));
}

export function canPickAsset(
  asset: PickerVideoAsset,
  allowTypes?: PickerVideoAssetType[],
) {
  return getAllowedAssetTypes(allowTypes).includes(asset.type);
}
</script>

<script setup lang="ts">
import type { VideoAsset, VideoAssetType } from '#/api';

import { computed, reactive, ref, watch } from 'vue';

import { useAccessStore } from '@vben/stores';

import {
  Button,
  Card,
  Empty,
  Image,
  Input,
  Modal,
  Segmented,
  Spin,
  Tag,
  message,
} from 'ant-design-vue';

import { listAssetsApi } from '#/api';

import {
  getAssetPreviewKind,
  getAssetPreviewSource,
  withPreviewToken,
} from '../asset-preview';

const props = defineProps<{
  // 限定可选的资产类型；不传则全部可选。用于参考图片/视频/音频分区限定来源。
  allowTypes?: VideoAssetType[];
  open: boolean;
}>();
const emit = defineEmits<{
  (e: 'update:open', value: boolean): void;
  (e: 'pick', asset: VideoAsset): void;
}>();

// 当前允许选择的类型列表（未限定则为全部四类）。
const allowed = computed<VideoAssetType[]>(() =>
  getAllowedAssetTypes(props.allowTypes),
);

const typeTabs = computed(() => [
  // 限定为单一类型时无需「全部」切换。
  ...(allowed.value.length > 1 ? [{ label: '全部', value: '' }] : []),
  ...allowed.value.map((value) => ({
    label: TYPE_META[value].label,
    value,
  })),
]);

const loading = ref(false);
const assets = ref<VideoAsset[]>([]);
const accessStore = useAccessStore();
const state = reactive({
  keyword: '',
  type: '' as '' | VideoAssetType,
});

const visible = computed({
  get: () => props.open,
  set: (value) => emit('update:open', value),
});

// PLACEHOLDER_SCRIPT
async function load() {
  loading.value = true;
  const nextType = normalizePickerQueryType(state.type, props.allowTypes);
  if (state.type !== nextType) {
    state.type = nextType;
  }
  try {
    const result = await listAssetsApi({
      keyword: state.keyword || undefined,
      page: 1,
      pageSize: 200,
      type: nextType || undefined,
    });
    assets.value = filterAllowedAssets(result.items, props.allowTypes);
  } finally {
    loading.value = false;
  }
}

function typeColor(type: VideoAssetType) {
  return TYPE_META[type]?.color ?? 'default';
}

function typeLabel(type: VideoAssetType) {
  return TYPE_META[type]?.label ?? type;
}

function previewKind(asset: VideoAsset) {
  return getAssetPreviewKind(asset.type, getAssetPreviewSource(asset));
}

function previewSource(asset: VideoAsset) {
  return withPreviewToken(
    getAssetPreviewSource(asset),
    accessStore.accessToken,
  );
}

function choose(asset: VideoAsset) {
  if (!canPickAsset(asset, props.allowTypes)) {
    message.error('该资产类型不在当前可选范围内');
    return;
  }
  emit('pick', asset);
  visible.value = false;
}

watch(
  () => props.open,
  (value) => {
    if (value) {
      state.type = getInitialPickerType(props.allowTypes);
      load();
    }
  },
);

watch(
  () => props.allowTypes,
  () => {
    state.type = getInitialPickerType(props.allowTypes);
    if (props.open) {
      load();
    }
  },
);

watch(
  () => state.type,
  () => {
    if (props.open) {
      load();
    }
  },
);
</script>

<template>
  <Modal v-model:open="visible" :footer="null" :width="760" title="资产库">
    <div class="picker-head">
      <Segmented v-model:value="state.type" :options="typeTabs" />
      <Input
        v-model:value="state.keyword"
        allow-clear
        class="picker-search"
        placeholder="搜索资产名称"
        @press-enter="load"
      />
      <Button type="primary" @click="load">查询</Button>
    </div>

    <Spin :spinning="loading">
      <div v-if="assets.length" class="picker-grid">
        <Card
          v-for="asset in assets"
          :key="asset.id"
          :body-style="{ padding: '10px' }"
          class="picker-card"
          hoverable
          @click="choose(asset)"
        >
          <div class="picker-media">
            <Image
              v-if="previewKind(asset) === 'image'"
              :preview="false"
              :src="previewSource(asset)"
            />
            <video
              v-else-if="previewKind(asset) === 'video'"
              :src="previewSource(asset)"
              muted
              preload="metadata"
            />
            <audio
              v-else-if="previewKind(asset) === 'audio'"
              :src="previewSource(asset)"
              controls
              @click.stop
            />
            <div v-else class="empty-tile">暂无预览</div>
          </div>
          <div class="picker-meta">
            <Tag :color="typeColor(asset.type)">
              {{ typeLabel(asset.type) }}
            </Tag>
            <span class="picker-name" :title="asset.name">{{
              asset.name
            }}</span>
          </div>
        </Card>
      </div>
      <Empty v-else description="暂无资产，请先在资产库中上传" />
    </Spin>
  </Modal>
</template>

<style scoped>
.picker-head {
  display: flex;
  gap: 12px;
  align-items: center;
  margin-bottom: 16px;
}

.picker-search {
  flex: 1;
}

.picker-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 12px;
  max-height: 440px;
  overflow-y: auto;
}

.picker-card {
  cursor: pointer;
  border-radius: 8px;
}

.picker-media {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 110px;
  overflow: hidden;
  background: #f1f5f9;
  border-radius: 6px;
}

.picker-media :deep(.ant-image),
.picker-media :deep(.ant-image-img),
.picker-media audio,
.picker-media video {
  width: 100%;
}

.picker-media :deep(.ant-image),
.picker-media :deep(.ant-image-img),
.picker-media video {
  height: 100%;
  object-fit: cover;
}

.picker-media audio {
  margin: 0 8px;
}

.empty-tile {
  font-size: 13px;
  color: #667085;
}

.picker-meta {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 8px;
}

.picker-name {
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
  color: #344054;
  white-space: nowrap;
}
</style>
