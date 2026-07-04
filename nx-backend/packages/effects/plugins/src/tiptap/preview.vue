<script setup lang="ts">
import type { TipTapPreviewProps } from './types';

import { computed } from 'vue';

import { cn } from '@vben-core/shared/utils';

import { sanitizeTipTapHTML } from './sanitize-html';

import './style.css';
const props = withDefaults(defineProps<TipTapPreviewProps>(), {
  content: '',
  minHeight: 160,
});
const contentMinHeight = computed(() =>
  typeof props.minHeight === 'number'
    ? `${props.minHeight}px`
    : props.minHeight,
);
const previewClass = computed(() =>
  cn(
    'vben-tiptap-content',
    'text-foreground bg-transparent p-0 leading-7',
    props.class,
  ),
);
const sanitizedContent = computed(() => sanitizeTipTapHTML(props.content));
</script>

<template>
  <!-- eslint-disable vue/no-v-html -->
  <div
    :class="previewClass"
    :style="{ minHeight: contentMinHeight }"
    v-html="sanitizedContent"
  ></div>
</template>
