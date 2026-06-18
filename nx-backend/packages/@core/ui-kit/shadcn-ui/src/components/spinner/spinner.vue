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
}

defineOptions({
  name: 'VbenSpinner',
});

const props = withDefaults(defineProps<Props>(), {
  minLoadingTime: 50,
});
// const startTime = ref(0);
const showSpinner = ref(false);
const renderSpinner = ref(false);
const timer = ref<ReturnType<typeof setTimeout>>();

watch(
  () => props.spinning,
  (show) => {
    if (!show) {
      showSpinner.value = false;
      clearTimeout(timer.value);
      return;
    }

    // startTime.value = performance.now();
    timer.value = setTimeout(() => {
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
        'flex-center bg-overlay-content absolute top-0 left-0 z-100 size-full backdrop-blur-xs transition-all duration-500',
        {
          'invisible opacity-0': !showSpinner,
        },
        props.class,
      )
    "
    @transitionend="onTransitionEnd"
  >
    <div
      :class="{ paused: !renderSpinner }"
      v-if="renderSpinner"
      class="brand-loader"
    >
      <img alt="loading" src="/logo.png" />
    </div>
  </div>
</template>

<style scoped>
.paused {
  &::before {
    animation-play-state: paused !important;
  }

  img {
    animation-play-state: paused !important;
  }
}

.brand-loader {
  position: relative;
  width: 64px;
  height: 64px;
}

.brand-loader::before {
  position: absolute;
  top: 74px;
  left: 50%;
  width: 48px;
  height: 6px;
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
  filter: drop-shadow(0 10px 18px rgb(0 0 0 / 24%));
  transform-origin: 50% 50%;
  animation: brand-float-ani 1.4s ease-in-out infinite;
}

@keyframes brand-float-ani {
  0%,
  100% {
    transform: translateY(0) rotate(-4deg) scale(1);
  }

  50% {
    transform: translateY(-10px) rotate(4deg) scale(1.04);
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
