<script setup>
import { computed } from 'vue'
import { typeTheme } from '../utils/typeTheme'

const props = defineProps({
  typeId: {
    type: [Number, String],
    required: true,
  },
  size: {
    type: String,
    default: 'md',
  },
  selected: {
    type: Boolean,
    default: false,
  },
  label: {
    type: String,
    default: '',
  },
  disabled: {
    type: Boolean,
    default: false,
  },
})

const theme = computed(() => typeTheme(props.typeId))
const themeStyle = computed(() => ({
  '--type-accent': theme.value.accent,
  '--type-soft': theme.value.soft,
  '--type-ink': theme.value.ink,
}))
</script>

<template>
  <view
    class="type-badge"
    :class="[
      `type-badge--${size}`,
      { 'type-badge--selected': selected, 'type-badge--disabled': disabled },
    ]"
    :style="themeStyle"
    :aria-disabled="disabled ? 'true' : 'false'"
    :aria-pressed="selected ? 'true' : 'false'"
  >
    <text class="type-badge__number">{{ typeId }}</text>
    <text v-if="label" class="type-badge__label">{{ label }}</text>
  </view>
</template>

<style scoped>
.type-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10rpx;
  min-height: 56rpx;
  padding: 8rpx 18rpx;
  border: 2rpx solid var(--type-soft);
  border-radius: 999rpx;
  background: var(--type-soft);
  color: var(--type-ink);
  box-sizing: border-box;
  transition: opacity .16s ease, transform .16s ease, box-shadow .16s ease;
}

.type-badge--sm { min-height: 48rpx; padding: 4rpx 14rpx; font-size: 22rpx; }
.type-badge--md { min-height: 56rpx; font-size: 24rpx; }
.type-badge--lg { min-height: 68rpx; padding: 10rpx 22rpx; font-size: 28rpx; }

.type-badge--selected {
  border-color: var(--type-accent);
  box-shadow: 0 0 0 4rpx var(--type-soft);
}

.type-badge--disabled {
  opacity: .45;
}

.type-badge__number {
  color: var(--type-accent);
  font-weight: 900;
}

.type-badge__label {
  font-weight: 700;
}

@media (prefers-reduced-motion: reduce) {
  .type-badge { transition: none; }
}
</style>
