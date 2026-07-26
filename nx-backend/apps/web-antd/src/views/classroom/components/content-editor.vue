<script setup lang="ts">
import type {
  ClassroomContent,
  ClassroomContentCreatePayload,
  ClassroomSeries,
} from '#/api/core/classroom';

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

const props = defineProps<{
  canPrice?: boolean;
  canWrite?: boolean;
  content?: ClassroomContent;
  series: ClassroomSeries[];
}>();
const emit = defineEmits<{
  change: [value: ClassroomContentCreatePayload];
  validity: [valid: boolean];
  saved: [value: ClassroomContent];
  cancel: [];
}>();

const form = reactive<ClassroomContentCreatePayload>({
  contentType: 'video',
  showAsStandalone: true,
  title: '',
});
const accessLevel = ref<'inherit' | 'public' | 'login' | 'member' | 'paid'>(
  'inherit',
);
const priceCents = ref(0);
const saving = ref(false);

watch(
  () => props.content,
  (content) => {
    Object.assign(
      form,
      content
        ? {
            badge: content.badge,
            contentType: content.contentType,
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
        : { contentType: 'video', showAsStandalone: true, title: '' },
    );
    accessLevel.value = content?.accessLevel ?? 'inherit';
    priceCents.value = content?.priceCents ?? 0;
  },
  { immediate: true },
);

async function save() {
  if (!form.title.trim()) return message.warning('请填写课件标题');
  if (standalonePaidPolicyError.value)
    return message.warning('请先明确独立付费课件的购买策略');
  if (accessLevel.value === 'paid' && priceCents.value < 1)
    return message.warning('请填写有效单课价格');
  saving.value = true;
  try {
    let saved = props.content
      ? await updateClassroomContentApi(props.content.id, {
          ...form,
          expectedUpdatedAt: props.content.updatedAt,
        })
      : await createClassroomContentApi({ ...form });
    if (
      props.canPrice &&
      (saved.accessLevel !== accessLevel.value ||
        saved.priceCents !== priceCents.value)
    ) {
      saved = await setClassroomContentPriceApi(saved.id, {
        accessLevel: accessLevel.value,
        expectedUpdatedAt: saved.updatedAt,
        priceCents: accessLevel.value === 'paid' ? priceCents.value : 0,
      });
    }
    message.success('课件已保存');
    emit('saved', saved);
  } finally {
    saving.value = false;
  }
}

const parentSeries = computed(() =>
  props.series.find((item) => item.id === form.seriesId),
);
const standalonePaidPolicyError = computed(() =>
  Boolean(
    form.showAsStandalone &&
    form.seriesId &&
    accessLevel.value === 'inherit' &&
    parentSeries.value?.accessLevel === 'paid',
  ),
);

watch(
  form,
  () => {
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
    <Form.Item label="课件标题" required
      ><Input v-model:value="form.title" placeholder="请输入清晰、可检索的标题"
    /></Form.Item>
    <Form.Item label="课件类型" required>
      <Radio.Group v-model:value="form.contentType" button-style="solid">
        <Radio.Button value="video">视频课件</Radio.Button
        ><Radio.Button value="audio">音频课件</Radio.Button>
      </Radio.Group>
    </Form.Item>
    <Form.Item label="内容入口">
      <Radio.Group v-model:value="form.showAsStandalone">
        <Radio :value="false">系列内容</Radio
        ><Radio :value="true">独立内容</Radio>
      </Radio.Group>
    </Form.Item>
    <Form.Item label="所属系列">
      <Select
        v-model:value="form.seriesId"
        allow-clear
        placeholder="不选择则为独立课件"
        :options="series.map((item) => ({ label: item.title, value: item.id }))"
      />
    </Form.Item>
    <Form.Item label="访问权限">
      <Select
        v-model:value="accessLevel"
        :disabled="!canPrice"
        :options="[
          { label: '继承系列', value: 'inherit' },
          { label: '公开', value: 'public' },
          { label: '登录后', value: 'login' },
          { label: '会员', value: 'member' },
          { label: '单课付费', value: 'paid' },
        ]"
      />
      <span v-if="!canPrice" class="field-hint"
        >当前账号没有定价权限，权限与价格仅可查看。</span
      >
    </Form.Item>
    <Form.Item v-if="accessLevel === 'paid'" label="单课价格（分）" required>
      <InputNumber
        v-model:value="priceCents"
        :disabled="!canPrice"
        :min="1"
        :precision="0"
        addon-after="分"
      />
    </Form.Item>
    <Alert
      v-if="standalonePaidPolicyError"
      type="warning"
      show-icon
      message="该课件以独立入口展示且继承付费系列；请明确采用“购买系列”策略，或将单课权限设为单课付费后再发布。"
    />
    <p v-else-if="form.showAsStandalone && form.seriesId" class="policy-hint">
      独立入口会展示购买策略：继承系列时购买系列；单课付费时购买本课。
    </p>
    <Form.Item label="老师名称"
      ><Input v-model:value="form.teacherName"
    /></Form.Item>
    <Form.Item label="简介"
      ><Input.TextArea v-model:value="form.description" :rows="4"
    /></Form.Item>
    <Form.Item label="排序"
      ><InputNumber v-model:value="form.sortOrder" :min="0"
    /></Form.Item>
    <Checkbox v-model:checked="form.showAsStandalone"
      >在独立内容入口展示</Checkbox
    >
    <div class="editor-actions">
      <Space
        ><Button @click="emit('cancel')">取消</Button
        ><Button
          type="primary"
          :loading="saving"
          :disabled="!canWrite"
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
