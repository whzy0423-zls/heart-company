export function userErrorMessage(error, fallback = '操作失败，请稍后重试') {
  const message = typeof error?.message === 'string' ? error.message.trim() : ''
  if (message) return message

  const errMsg = typeof error?.errMsg === 'string' ? error.errMsg.toLowerCase() : ''
  if (errMsg.includes('cancel')) {
    return errMsg.includes('requestpayment') ? '已取消支付' : '已取消操作'
  }

  return fallback
}
