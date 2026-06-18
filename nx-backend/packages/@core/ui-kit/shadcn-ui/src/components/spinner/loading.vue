<script lang="ts" setup>
import { ref, watch } from 'vue';

import { cn } from '@vben-core/shared/utils';

interface Props {
  class?: string;
  /**
   * @zh_CN 最小加载时间
   * @en_US Minimum loading time
   */
  minLoadingTime?: number;

  /**
   * @zh_CN loading状态开启
   */
  spinning?: boolean;
  /**
   * @zh_CN 文字
   */
  text?: string;
}

defineOptions({
  name: 'VbenLoading',
});

const props = withDefaults(defineProps<Props>(), {
  minLoadingTime: 50,
  text: '',
});
// const startTime = ref(0);
const showSpinner = ref(false);
const renderSpinner = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;

watch(
  () => props.spinning,
  (show) => {
    if (!show) {
      showSpinner.value = false;
      timer && clearTimeout(timer);
      return;
    }

    // startTime.value = performance.now();
    timer = setTimeout(() => {
      // const loadingTime = performance.now() - startTime.value;

      showSpinner.value = true;
      if (showSpinner.value) {
        renderSpinner.value = true;
      }
    }, props.minLoadingTime);
  },
  {
    immediate: true,
  },
);

function onTransitionEnd() {
  if (!showSpinner.value) {
    renderSpinner.value = false;
  }
}
</script>

<template>
  <div
    :class="
      cn(
        'bg-overlay-content dark:bg-overlay absolute top-0 left-0 z-100 flex size-full flex-col items-center justify-center transition-all duration-500',
        {
          'invisible opacity-0': !showSpinner,
        },
        props.class,
      )
    "
    @transitionend="onTransitionEnd"
  >
    <slot name="icon" v-if="renderSpinner">
      <span class="brand-loader">
        <img alt="loading" src="/logo.png" />
      </span>
    </slot>

    <div v-if="text" class="text-primary mt-4 text-xs">{{ text }}</div>
    <slot></slot>
  </div>
</template>

<style scoped>
.brand-loader {
  position: relative;
  display: inline-block;
  width: 54px;
  height: 54px;
}

.brand-loader::before {
  position: absolute;
  top: 62px;
  left: 50%;
  width: 40px;
  height: 5px;
  content: '';
  background: hsl(var(--primary) / 35%);
  border-radius: 50%;
  filter: blur(1px);
  transform: translateX(-50%);
  animation: brand-shadow-ani 1.4s ease-in-out infinite;
}

.brand-loader img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  filter: drop-shadow(0 8px 14px rgb(0 0 0 / 22%));
  transform-origin: 50% 50%;
  animation: brand-float-ani 1.4s ease-in-out infinite;
}

@keyframes brand-float-ani {
  0%,
  100% {
    transform: translateY(0) rotate(-4deg) scale(1);
  }

  50% {
    transform: translateY(-8px) rotate(4deg) scale(1.04);
  }
}

@keyframes brand-shadow-ani {
  0%,
  100% {
    opacity: 0.45;
    transform: translateX(-50%) scale(0.88, 1);
  }

  50% {
    opacity: 0.25;
    transform: translateX(-50%) scale(1.16, 1);
  }
}
</style>
