import { TYPES_INFO, CENTERS, GENDER_WEIGHT } from '../data/enneagramGame'

// 计算结果：累加权重 + 性别小数决胜；返回主型/副型/三中心分布。
export function calcType(answers, gender) {
  const score = {}
  for (let i = 1; i <= 9; i++) score[i] = 0
  answers.forEach((opt) => {
    if (!opt) return
    Object.entries(opt.w).forEach(([id, v]) => { score[id] += v })
  })
  const gw = GENDER_WEIGHT[gender] || {}
  const adjusted = {}
  for (let id = 1; id <= 9; id++) {
    adjusted[id] = score[id] + (score[id] * (gw[id] || 1) - score[id]) * 0.15
  }
  const ranking = Object.keys(adjusted)
    .map((id) => ({ id: Number(id), raw: score[id], val: adjusted[id] }))
    .sort((a, b) => b.val - a.val)
  const best = ranking[0].id
  const second = ranking.find((r) => r.id !== best && r.raw > 0)?.id || null

  const centerScore = { gut: 0, heart: 0, head: 0 }
  for (let id = 1; id <= 9; id++) centerScore[TYPES_INFO[id].center] += score[id]
  const centerTotal = centerScore.gut + centerScore.heart + centerScore.head || 1
  const centers = ['gut', 'heart', 'head'].map((key) => ({
    key,
    name: CENTERS[key].name,
    pct: Math.round((centerScore[key] / centerTotal) * 100),
  }))

  return { type: best, second, score, centers }
}

export function isWing(main, other) {
  if (!other) return false
  const diff = Math.abs(main - other)
  return diff === 1 || diff === 8
}
