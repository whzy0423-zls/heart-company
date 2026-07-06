<script setup lang="ts">
import { computed, useAttrs } from 'vue';

import { Tooltip } from 'ant-design-vue';

defineOptions({ inheritAttrs: false });

const attrs = useAttrs();

const props = withDefaults(
  defineProps<{
    emptyText?: string;
    lines?: number;
    maxWidth?: number | string;
    placement?: 'bottom' | 'left' | 'right' | 'top' | 'topLeft' | 'topRight';
    text?: null | number | string;
  }>(),
  {
    emptyText: '-',
    lines: 1,
    maxWidth: 520,
    placement: 'topLeft',
    text: '',
  },
);

const displayText = computed(() => {
  const value = props.text === null || props.text === undefined ? '' : String(props.text);
  return value.trim() ? value : props.emptyText;
});

const textClass = computed(() =>
  props.lines > 1 ? 'ellipsis-tooltip__text--multi' : 'ellipsis-tooltip__text--single',
);

const textStyle = computed(() => ({
  '--ellipsis-lines': String(Math.max(1, props.lines)),
}));

const textAttrs = computed(() => {
  const { class: _class, style: _style, ...rest } = attrs;
  return rest;
});

const mergedTextClass = computed(() => [textClass.value, attrs.class]);
const mergedTextStyle = computed(() => [textStyle.value, attrs.style]);

const overlayStyle = computed(() => ({
  maxWidth: typeof props.maxWidth === 'number' ? `${props.maxWidth}px` : props.maxWidth,
  whiteSpace: 'pre-wrap',
}));
</script>

<template>
  <span
    v-bind="textAttrs"
    class="ellipsis-tooltip__text"
    :class="mergedTextClass"
    :style="mergedTextStyle"
  >
    <Tooltip :overlay-style="overlayStyle" :placement="placement" :title="displayText">
      <span class="ellipsis-tooltip__content">{{ displayText }}</span>
    </Tooltip>
  </span>
</template>

<style scoped>
.ellipsis-tooltip__text {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  line-height: 1.6;
  overflow-wrap: anywhere;
  vertical-align: middle;
}

.ellipsis-tooltip__content {
  min-width: 0;
}

.ellipsis-tooltip__text--single {
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ellipsis-tooltip__text--multi {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: var(--ellipsis-lines);
  white-space: normal;
}
</style>
