export function copyText(text, deps = {}) {
  const value = String(text || '').trim()
  const clipboard = deps.clipboard || ((options) => uni.setClipboardData(options))
  const toast = deps.toast || ((options) => uni.showToast(options))

  if (!value) {
    toast({ title: '没有可复制内容', icon: 'none' })
    return Promise.resolve(false)
  }

  return new Promise((resolve) => {
    clipboard({
      data: value,
      success: () => {
        toast({ title: '已复制答案', icon: 'success' })
        resolve(true)
      },
      fail: () => {
        toast({ title: '复制失败，请重试', icon: 'none' })
        resolve(false)
      },
    })
  })
}
