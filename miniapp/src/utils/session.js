// 跨页传递最近一次测试结果（mp-weixin 单 JS 上下文，模块单例可用）。
let lastResult = null
let lastGender = null

export function setLastResult(result, gender) {
  lastResult = result
  lastGender = gender
}

export function getLastResult() {
  return { result: lastResult, gender: lastGender }
}
