import { computed, ref } from 'vue'
import { onResize } from '@dcloudio/uni-app'
import { resolveHomeNavigation } from './homeNavigation'

export const STUDIO_HEADER_HEIGHT_RPX = '144rpx'

function getRuntimeApi() {
  // `uni` is injected by the app runtime in a normal page, but keeping this
  // lookup indirect also lets the shared header be evaluated by preview and
  // test harnesses that do not install a global bridge yet.
  if (typeof uni !== 'undefined') return uni
  if (typeof globalThis !== 'undefined' && globalThis.uni) return globalThis.uni
  return undefined
}

/**
 * Shared geometry for the four tab pages that use the studio chrome.
 * Keeping the status-bar measurement in one place prevents one tab from
 * falling behind the home page when the simulator rotates or reports a late
 * native window measurement.
 */
export function useStudioNavigation() {
  let wechat = false
  // #ifdef MP-WEIXIN
  wechat = true
  // #endif

  const navigation = ref(resolveHomeNavigation(getRuntimeApi(), { wechat }))
  const statusBarHeight = computed(() => navigation.value.statusBarHeight)
  const headerStyle = computed(() => ({ paddingTop: `${statusBarHeight.value}px` }))
  const topbarStyle = computed(() => navigation.value.capsuleInset
    ? { paddingRight: `${navigation.value.capsuleInset}px` }
    : {})
  const pageStyle = computed(() => ({
    paddingTop: `calc(${statusBarHeight.value}px + ${STUDIO_HEADER_HEIGHT_RPX})`,
  }))

  function refreshNavigation() {
    navigation.value = resolveHomeNavigation(getRuntimeApi(), { wechat })
  }

  onResize(refreshNavigation)

  return { navigation, statusBarHeight, headerStyle, topbarStyle, pageStyle, refreshNavigation }
}
