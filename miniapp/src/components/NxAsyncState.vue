<template>
  <view
    class="nx-async-state"
    :class="`nx-async-state--${state}`"
    role="status"
    aria-live="polite"
  >
    <view v-if="state === 'loading'" class="nx-async-state__loading" aria-label="加载中">
      <view class="nx-async-state__spinner" aria-hidden="true" />
      <text class="nx-async-state__loading-text">加载中…</text>
    </view>

    <template v-else>
      <text v-if="title" class="nx-async-state__title">{{ title }}</text>
      <text v-if="description" class="nx-async-state__description">{{ description }}</text>
      <button
        v-if="actionText"
        class="nx-button nx-button--secondary nx-async-state__action"
        :disabled="busy"
        :aria-label="busy ? '处理中…' : actionText"
        @click="handleAction"
      >
        {{ busy ? '处理中…' : actionText }}
      </button>
    </template>
  </view>
</template>

<script setup>
const props = defineProps({
  state: {
    type: String,
    required: true,
    validator: (value) => ['loading', 'stale', 'empty', 'error'].includes(value),
  },
  title: {
    type: String,
    default: '',
  },
  description: {
    type: String,
    default: '',
  },
  actionText: {
    type: String,
    default: '',
  },
  busy: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['action'])

function handleAction() {
  if (props.busy || !props.actionText) return
  emit('action')
}
</script>

<style scoped>
.nx-async-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 16rpx;
  padding: 40rpx 32rpx;
  border-radius: 24rpx;
  background: #f8fafc;
  color: #334155;
  text-align: center;
}

.nx-async-state--loading { background: #eff6ff; color: #1d4ed8; }
.nx-async-state--stale { background: #fffbeb; color: #92400e; }
.nx-async-state--empty { background: #f8fafc; color: #475569; }
.nx-async-state--error { background: #fef2f2; color: #b91c1c; }

.nx-async-state__loading {
  display: flex;
  align-items: center;
  gap: 16rpx;
}

.nx-async-state__spinner {
  width: 32rpx;
  height: 32rpx;
  border: 4rpx solid currentColor;
  border-right-color: transparent;
  border-radius: 50%;
  animation: nx-async-state-spin 800ms linear infinite;
}

.nx-async-state__loading-text,
.nx-async-state__description {
  font-size: 26rpx;
  line-height: 1.6;
}

.nx-async-state__title {
  font-size: 30rpx;
  font-weight: 700;
  line-height: 1.4;
}

.nx-async-state__action {
  min-height: 88rpx;
  margin-top: 8rpx;
}

@media (prefers-reduced-motion: reduce) {
  .nx-async-state__spinner {
    animation: none;
  }
}

@keyframes nx-async-state-spin {
  to { transform: rotate(360deg); }
}
</style>
