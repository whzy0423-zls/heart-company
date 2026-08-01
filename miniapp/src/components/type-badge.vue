<script setup>
import { computed } from 'vue'
import { typeTheme } from '../utils/typeTheme'
import { handleTypeBadgeClick } from './typeBadgeInteraction'

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
  interactive: {
    type: Boolean,
    default: false,
  },
})
const emit = defineEmits(['click'])

const theme = computed(() => typeTheme(props.typeId))
const accessibleLabel = computed(() => (
  props.label
    ? `${props.typeId}号类型，${props.label}`
    : `${props.typeId}号类型`
))
const themeStyle = computed(() => ({
  '--type-accent': theme.value.accent,
  '--type-soft': theme.value.soft,
  '--type-ink': theme.value.ink,
}))

function onClick(event) {
  return handleTypeBadgeClick(props.interactive, props.disabled, emit, event)
}
</script>

<template>
  <view
    class="type-badge-hit"
    :class="{
      'type-badge-hit--interactive': interactive,
      'type-badge-hit--disabled': interactive && disabled,
    }"
    :style="themeStyle"
    :role="interactive ? 'button' : undefined"
    :aria-label="interactive ? accessibleLabel : undefined"
    :aria-disabled="interactive ? (disabled ? 'true' : 'false') : undefined"
    :aria-pressed="interactive ? (selected ? 'true' : 'false') : undefined"
    :tabindex="interactive && !disabled ? 0 : undefined"
    @click="onClick"
    @keydown.enter.prevent="onClick"
    @keydown.space.prevent="onClick"
  >
    <view
      class="type-badge"
      :class="[
        `type-badge--${size}`,
        { 'type-badge--selected': selected, 'type-badge--disabled': disabled },
      ]"
    >
      <text class="type-badge__number">{{ typeId }}</text>
      <text v-if="label" class="type-badge__label">{{ label }}</text>
    </view>
  </view>
</template>

<style scoped>
.type-badge-hit {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  vertical-align: middle;
  box-sizing: border-box;
}

.type-badge-hit--interactive {
  min-width: 88rpx;
  min-height: 88rpx;
  padding: 10rpx;
  border-radius: 999rpx;
  cursor: pointer;
  touch-action: manipulation;
}

.type-badge-hit--interactive:active .type-badge {
  transform: scale(.97);
}

.type-badge-hit--interactive:focus-visible {
  outline: 4rpx solid var(--nx-focus, #2449C7);
  outline-offset: 2rpx;
}

.type-badge-hit--disabled {
  cursor: default;
}

.type-badge {
  --badge-ink: var(--nx-ink, #17212B);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 10rpx;
  min-height: 56rpx;
  padding: 8rpx 18rpx;
  border: 2rpx solid var(--nx-line, #DEDCD5);
  border-radius: 999rpx;
  background: var(--nx-surface, #FFFDF8);
  color: var(--badge-ink);
  box-sizing: border-box;
  transition: opacity .16s ease, transform .16s ease, box-shadow .16s ease;
}

.type-badge--sm { min-height: 48rpx; padding: 4rpx 14rpx; font-size: 22rpx; }
.type-badge--md { min-height: 56rpx; font-size: 24rpx; }
.type-badge--lg { min-height: 68rpx; padding: 10rpx 22rpx; font-size: 28rpx; }

.type-badge--selected {
  border-color: var(--type-accent);
  background: var(--type-soft);
  box-shadow: inset 0 0 0 4rpx var(--type-accent);
}

.type-badge--disabled {
  opacity: .45;
}

.type-badge__number {
  color: var(--badge-ink);
  font-weight: 900;
}

.type-badge__label {
  color: var(--badge-ink);
  font-weight: 700;
}

@media (prefers-reduced-motion: reduce) {
  .type-badge { transition: none; }
}
</style>
