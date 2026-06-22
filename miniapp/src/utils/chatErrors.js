export function chatErrorMessage(error) {
  const message = error && error.message ? error.message : ''
  if (message.includes('微信登录未返回 code')) {
    return '微信登录没有拿到授权 code，请稍后重试，或重新打开小程序。'
  }
  if (error && (error.statusCode === 401 || error.statusCode === 403)) {
    return '登录状态已失效，请重新进入页面后再试。'
  }
  if (error && error.statusCode === 429) {
    return '提问有点太频繁了，稍等一分钟再继续问。'
  }
  if (message.includes('deadline exceeded') || message.includes('超时')) {
    return '本次回答生成超时了，可以稍后再试，或把问题问得更具体一点。'
  }
  return '刚才没有连接上对话服务，请稍后再试，或换个更具体的问题。'
}
