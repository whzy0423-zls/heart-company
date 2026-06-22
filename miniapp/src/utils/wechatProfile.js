function normalizeGender(value) {
  if (value === 1 || value === '1' || value === 'male') return 'male'
  if (value === 2 || value === '2' || value === 'female') return 'female'
  return ''
}

function clean(value) {
  return String(value || '').trim()
}

export function normalizeWechatProfile(source = {}) {
  const info = source.userInfo || source
  const payload = {}
  const nickname = clean(info.nickName || info.nickname)
  const avatar = clean(info.avatarUrl || info.avatar)
  const gender = normalizeGender(info.gender)

  if (nickname) payload.nickname = nickname
  if (avatar) payload.avatar = avatar
  if (gender) payload.gender = gender

  return payload
}

export function hasProfilePayload(payload = {}) {
  return Boolean(payload.nickname || payload.avatar || payload.gender)
}

export function getWechatProfilePayload(desc = '用于完善九型档案') {
  if (typeof uni === 'undefined' || !uni.getUserProfile) {
    return Promise.resolve({})
  }

  return new Promise((resolve, reject) => {
    uni.getUserProfile({
      desc,
      success: (res) => resolve(normalizeWechatProfile(res)),
      fail: reject,
    })
  })
}
