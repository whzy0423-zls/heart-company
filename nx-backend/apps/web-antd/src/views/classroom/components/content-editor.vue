<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomContentCreatePayload,
  ClassroomSeries,
} from '#/api/core/classroom';
import type { TeacherProfile } from '#/api/core/teacher';

import { computed, reactive, ref, watch } from 'vue';
import {
  Alert,
  Button,
  Checkbox,
  Form,
  Input,
  InputNumber,
  Radio,
  Select,
  Space,
  message,
} from 'ant-design-vue';
import {
  createClassroomContentApi,
  setClassroomContentPriceApi,
  updateClassroomContentApi,
} from '#/api/core/classroom';
import {
  contentMetadataPayload,
  createContentDraftDefaults,
  purchaseStrategyRequired,
  saveContentWorkflow,
} from '../editor-model';

const props = defineProps<{
  canPrice?: boolean;
  canWrite?: boolean;
  content?: ClassroomContent;
  series: ClassroomSeries[];
  teachers?: TeacherProfile[];
}>();
const emit = defineEmits<{
  change: [value: ClassroomContentCreatePayload];
  validity: [valid: boolean];
  saved: [value: ClassroomContent];
  cancel: [];
}>();

const form = reactive<ClassroomContentCreatePayload>({
  ...contentMetadataPayload(createContentDraftDefaults()),
});
const accessLevel = ref<'inherit' | 'public' | 'login' | 'member' | 'paid'>(
  'public',
);
const priceCents = ref(0);
const likeCount = ref(0);
const favoriteCount = ref(0);
const saving = ref(false);
const standalonePaidStrategy = ref<'content' | 'series'>('series');
const persistedContent = ref<ClassroomContent>();
const metadataCommitted = ref(false);
const isDraftContent = computed(
  () => !props.content || props.content.status === 'draft',
);
const metadataEditable = computed(
  () => Boolean(props.canWrite) && isDraftContent.value,
);
const priceEditable = computed(() => Boolean(props.canPrice));
const teacherOptions = computed(() => {
  const options = (props.teachers ?? []).map((teacher) => ({
    label: `${teacher.name}${teacher.title ? ` · ${teacher.title}` : ''}`,
    value: teacher.key,
  }));
  if (form.teacherKey && !options.some((item) => item.value === form.teacherKey)) {
    options.unshift({
      label: `${form.teacherName || form.teacherKey}（当前资料）`,
      value: form.teacherKey,
    });
  }
  return options;
});
function selectTeacher(value: unknown) {
  const key = typeof value === 'string' || typeof value === 'number' ? String(value) : '';
  const teacher = (props.teachers ?? []).find((item) => item.key === key);
  form.teacherKey = key;
  form.teacherName = teacher?.name ?? '';
}

watch(
  () => props.content,
  (content) => {
    for (const key of Object.keys(form))
      delete (form as Record<string, unknown>)[key];
    Object.assign(
      form,
      content
        ? {
            badge: content.badge,
            contentType: content.contentType,
            coverAspectRatio: content.coverAspectRatio || '16:9',
            coverUrl: content.coverUrl,
            description: content.description,
            durationSeconds: content.durationSeconds,
            episodeNo: content.episodeNo,
            recordedAt: content.recordedAt,
            seriesId: content.seriesId,
            showAsStandalone: content.showAsStandalone,
            sortOrder: content.sortOrder,
            tags: content.tags,
            teacherKey: content.teacherKey,
            teacherName: content.teacherName,
            title: content.title,
          }
        : contentMetadataPayload(createContentDraftDefaults()),
    );
    accessLevel.value = content?.accessLevel ?? 'public';
    priceCents.value = content?.priceCents ?? 0;
    likeCount.value = content?.likeCount ?? 0;
    favoriteCount.value = content?.favoriteCount ?? 0;
    persistedContent.value = content;
    metadataCommitted.value = Boolean(content);
  },
  { immediate: true },
);

async function save() {
  if (!form.title.trim()) return message.warning('请填写课件标题');
  if (!props.canWrite && !props.content) return;
  if (accessLevel.value === 'paid' && priceCents.value < 1)
    return message.warning('请填写有效单课价格');
  saving.value = true;
  try {
    const saved = await saveContentWorkflow({
      create: () => createClassroomContentApi(contentMetadataPayload(form)),
      current: persistedContent.value,
      metadataCommitted: !props.canWrite || metadataCommitted.value,
      update: () =>
        updateClassroomContentApi(persistedContent.value!.id, {
          ...contentMetadataPayload(form),
          likeCount: Math.max(0, likeCount.value),
          favoriteCount: Math.max(0, favoriteCount.value),
          expectedUpdatedAt: persistedContent.value!.updatedAt,
        }),
      onPersist: (value) => {
        persistedContent.value = value;
        metadataCommitted.value = true;
      },
      price: props.canPrice
        ? () => {
            const current = persistedContent.value!;
            if (
              current.accessLevel === accessLevel.value &&
              current.priceCents === priceCents.value
            )
              return Promise.resolve(current);
            return setClassroomContentPriceApi(current.id, {
              accessLevel: accessLevel.value,
              expectedUpdatedAt: current.updatedAt,
              priceCents: accessLevel.value === 'paid' ? priceCents.value : 0,
            });
          }
        : undefined,
    });
    message.success('课件已保存');
    emit('saved', saved);
  } catch (cause) {
    message.error(cause instanceof Error ? cause.message : '课件保存失败');
  } finally {
    saving.value = false;
  }
}

const parentSeries = computed(() =>
  props.series.find((item) => item.id === form.seriesId),
);
const standalonePaidPolicyError = computed(
  () =>
    purchaseStrategyRequired(
      {
        accessLevel: accessLevel.value,
        seriesId: form.seriesId,
        showAsStandalone: Boolean(form.showAsStandalone),
      },
      parentSeries.value,
    ) && !standalonePaidStrategy.value,
);

watch(
  () => form.seriesId,
  (seriesId) => {
    if (!seriesId) {
      form.showAsStandalone = false;
      if (accessLevel.value === 'inherit') accessLevel.value = 'public';
    }
  },
);
watch(accessLevel, (value) => {
  if (value === 'inherit' && !form.seriesId) accessLevel.value = 'public';
});

watch(
  form,
  () => {
    if (metadataEditable.value) metadataCommitted.value = false;
    emit('change', { ...form });
    emit(
      'validity',
      Boolean(form.title.trim()) && !standalonePaidPolicyError.value,
    );
  },
  { deep: true, immediate: true },
);
</script>

<template>
  <Form layout="vertical" class="editor-form">
    <Alert
      v-if="!content"
      type="info"
      show-icon
      message="请先保存课件，再管理封面"
      description="封面文件需要绑定已保存的课件 ID。保存草稿后，可在列表行点击“封面管理”。"
    />
    <Alert
      v-else-if="content.status !== 'draft'"
      type="info"
      show-icon
      message="当前课件不是草稿，普通元数据已锁定"
      description="仅草稿可编辑课件元数据；如需调整封面，请在列表行使用“封面管理”。"
    />
    <Form.Item label="课件标题" required
      ><Input
        v-model:value="form.title"
        :disabled="!metadataEditable"
        placeholder="请输入清晰、可检索的标题"
    /></Form.Item>
    <Form.Item label="课件类型" required>
      <Radio.Group
        v-model:value="form.contentType"
        :disabled="!metadataEditable"
        button-style="solid"
      >
        <Radio.Button value="video">视频课件</Radio.Button
        ><Radio.Button value="audio">音频课件</Radio.Button>
      </Radio.Group>
    </Form.Item>
    <Space style="width: 100%" :size="16" wrap>
      <Form.Item label="点赞数"><InputNumber v-model:value="likeCount" :min="0" /></Form.Item>
      <Form.Item label="收藏数"><InputNumber v-model:value="favoriteCount" :min="0" /></Form.Item>
    </Space>
    <Form.Item label="所属老师视频系列">
      <Select
        v-model:value="form.seriesId"
        :disabled="!metadataEditable"
        allow-clear
        :placeholder="
          series.length
            ? '可选：选择已有老师视频系列'
            : '暂无老师视频系列，可直接保存为独立课件'
        "
        not-found-content="暂无老师视频系列，可直接保存为独立课件"
        :options="series.map((item) => ({ label: item.title, value: item.id }))"
      />
      <span class="field-hint">不加入系列，课件会独立展示；以后也可以再编辑归入系列。</span>
    </Form.Item>
    <Form.Item v-if="form.seriesId" label="展示入口">
      <Checkbox
        v-model:checked="form.showAsStandalone"
        :disabled="!metadataEditable"
        >同时在独立内容入口展示</Checkbox
      >
      <span class="field-hint"
        >关闭时仅作为视频系列内容展示；独立课件无需开启此项。</span
      >
    </Form.Item>
    <Form.Item label="访问权限">
      <Select
        v-model:value="accessLevel"
        :disabled="!priceEditable"
        :options="[
          { label: '继承系列', value: 'inherit', disabled: !form.seriesId },
          { label: '公开', value: 'public' },
          { label: '登录后', value: 'login' },
          { label: '会员', value: 'member' },
          { label: '单课付费', value: 'paid' },
        ]"
       placeholder="请选择访问权限"/>
      <span v-if="!priceEditable" class="field-hint"
        >当前账号没有定价权限，权限与价格仅可查看。</span
      >
    </Form.Item>
    <Form.Item v-if="accessLevel === 'paid'" label="单课价格（分）" required>
      <InputNumber
        v-model:value="priceCents"
        :disabled="!priceEditable"
        :min="1"
        :precision="0"
        addon-after="分"
       placeholder="请输入单课价格（分）"/>
    </Form.Item>
    <Alert
      v-if="
        purchaseStrategyRequired(
          {
            accessLevel,
            seriesId: form.seriesId,
            showAsStandalone: Boolean(form.showAsStandalone),
          },
          parentSeries,
        )
      "
      type="info"
      show-icon
      message="该课件继承付费系列并在独立入口展示，请确认用户购买策略。"
      ><template #description
        ><Radio.Group
          v-model:value="standalonePaidStrategy"
          :disabled="!metadataEditable"
          ><Radio value="series">购买系列（推荐，解锁系列全部课件）</Radio
          ><Radio value="content" disabled
            >单课付费（请先将访问权限切换为单课付费）</Radio
          ></Radio.Group
        ></template
      ></Alert
    >
    <p v-else-if="form.showAsStandalone && form.seriesId" class="policy-hint">
      独立入口会展示购买策略：继承系列时购买系列；单课付费时购买本课。
    </p>
    <Form.Item label="关联老师">
      <Select
        :value="form.teacherKey"
        :disabled="!metadataEditable"
        allow-clear
        :options="teacherOptions"
        placeholder="可选：从老师管理中选择"
        @update:value="selectTeacher"
      />
      <span class="field-hint">选择后会自动绑定老师标识；未选择时课件仍可作为通用课堂内容保存。</span>
    </Form.Item>
    <Form.Item label="简介"
      ><Input.TextArea
        v-model:value="form.description"
        :disabled="!metadataEditable"
        :rows="4"
     placeholder="请输入简介"/></Form.Item>
    <Form.Item label="排序"
      ><InputNumber
        v-model:value="form.sortOrder"
        :disabled="!metadataEditable"
        :min="0"
     placeholder="请输入排序"/></Form.Item>
    <div class="editor-actions">
      <Space
        ><Button @click="emit('cancel')">取消</Button
        ><Button
          type="primary"
          :loading="saving"
          :disabled="saving || (!metadataEditable && !priceEditable)"
          @click="save"
          >保存课件</Button
        ></Space
      >
    </div>
  </Form>
</template>

<style scoped>
.editor-form {
  max-width: 720px;
}
.policy-hint {
  color: hsl(var(--muted-foreground));
  line-height: 1.6;
}
.field-hint {
  display: block;
  color: hsl(var(--muted-foreground));
  margin-top: 4px;
}
.editor-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 24px;
}
</style>
