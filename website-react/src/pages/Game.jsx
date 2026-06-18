import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import QRCode from 'qrcode'
import Confetti from '../components/Confetti'
import { QUESTIONS, RESULTS, TYPES_INFO, CENTERS, GENDER_WEIGHT } from '../data/enneagramGame'
import { trackGameResult } from '../api/analytics'

const TYPE_HEX = { green: '#38a83a', blue: '#1f73c4', red: '#e23a2f' }

function loadImage(src) {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onload = () => resolve(img)
    img.onerror = reject
    img.src = src
  })
}

// 按字符宽度折行（中文友好）
function wrapText(ctx, text, maxWidth) {
  const lines = []
  let line = ''
  for (const ch of text) {
    const test = line + ch
    if (ctx.measureText(test).width > maxWidth && line) {
      lines.push(line)
      line = ch
    } else {
      line = test
    }
  }
  if (line) lines.push(line)
  return lines
}

// 生成「简洁分享卡片」：头像 + 型号 + 一句话 summary + 二维码引流回官网
async function buildShareCard(result, r, info) {
  const scale = 2
  const W = 640
  const H = 900
  const canvas = document.createElement('canvas')
  canvas.width = W * scale
  canvas.height = H * scale
  const ctx = canvas.getContext('2d')
  ctx.scale(scale, scale)
  const accent = TYPE_HEX[info.color] || '#1f73c4'
  const FF = '"PingFang SC", "Microsoft YaHei", sans-serif'

  // 背景
  const bg = ctx.createLinearGradient(0, 0, 0, H)
  bg.addColorStop(0, '#ffffff')
  bg.addColorStop(1, '#eef2f6')
  ctx.fillStyle = bg
  ctx.fillRect(0, 0, W, H)
  ctx.textAlign = 'center'

  // 顶部品牌
  ctx.fillStyle = '#9aa7b5'
  ctx.font = `600 18px ${FF}`
  ctx.fillText('九型芯之力 · 性格芯片测试', W / 2, 58)

  // 头像
  const cx = W / 2
  const cy = 215
  const rad = 86
  ctx.beginPath()
  ctx.arc(cx, cy, rad + 9, 0, Math.PI * 2)
  ctx.fillStyle = `${accent}22`
  ctx.fill()
  try {
    const avatar = await loadImage(`/assets/avatars/${result.type}.png`)
    ctx.save()
    ctx.beginPath()
    ctx.arc(cx, cy, rad, 0, Math.PI * 2)
    ctx.clip()
    ctx.drawImage(avatar, cx - rad, cy - rad, rad * 2, rad * 2)
    ctx.restore()
  } catch {
    ctx.beginPath()
    ctx.arc(cx, cy, rad, 0, Math.PI * 2)
    ctx.fillStyle = accent
    ctx.fill()
    ctx.fillStyle = '#fff'
    ctx.font = `800 72px ${FF}`
    ctx.textBaseline = 'middle'
    ctx.fillText(String(result.type), cx, cy + 4)
    ctx.textBaseline = 'alphabetic'
  }
  ctx.beginPath()
  ctx.arc(cx, cy, rad, 0, Math.PI * 2)
  ctx.lineWidth = 4
  ctx.strokeStyle = accent
  ctx.stroke()
  // 数字徽标
  ctx.beginPath()
  ctx.arc(cx + rad - 4, cy - rad + 4, 25, 0, Math.PI * 2)
  ctx.fillStyle = accent
  ctx.fill()
  ctx.fillStyle = '#fff'
  ctx.font = `800 28px ${FF}`
  ctx.textBaseline = 'middle'
  ctx.fillText(String(result.type), cx + rad - 4, cy - rad + 6)
  ctx.textBaseline = 'alphabetic'

  // 标题
  ctx.fillStyle = '#1a2430'
  ctx.font = `800 34px ${FF}`
  ctx.fillText(r.title, W / 2, cy + 152)
  // 英文 + 关键词
  ctx.fillStyle = accent
  ctx.font = `700 16px ${FF}`
  ctx.fillText(`${info.en} · ${info.keywords}`, W / 2, cy + 184)

  // 一句话 summary（折行）
  ctx.fillStyle = '#42505e'
  ctx.font = `400 20px ${FF}`
  const lines = wrapText(ctx, r.summary, W - 120)
  let ty = cy + 232
  lines.forEach((l) => {
    ctx.fillText(l, W / 2, ty)
    ty += 32
  })

  // 分隔线
  ctx.strokeStyle = '#e3e8ee'
  ctx.lineWidth = 1
  ctx.beginPath()
  ctx.moveTo(70, H - 278)
  ctx.lineTo(W - 70, H - 278)
  ctx.stroke()

  // 二维码
  const shareUrl = `${typeof window !== 'undefined' ? window.location.origin : ''}/game`
  const qrDataUrl = await QRCode.toDataURL(shareUrl, {
    width: 320,
    margin: 1,
    color: { dark: '#1a2430', light: '#ffffff' },
  })
  const qr = await loadImage(qrDataUrl)
  const qrSize = 142
  ctx.drawImage(qr, W / 2 - qrSize / 2, H - 250, qrSize, qrSize)
  ctx.fillStyle = '#6b7787'
  ctx.font = `600 16px ${FF}`
  ctx.fillText('扫码 · 测测你是哪一块性格芯片', W / 2, H - 78)
  // 品牌标语
  ctx.fillStyle = accent
  ctx.font = `700 16px ${FF}`
  ctx.fillText('九型芯之力 · 读懂性格，激活成长', W / 2, H - 46)

  return canvas.toDataURL('image/png')
}

// 计算结果型号：累加各题选项权重，再乘以极小的性别加权决胜。
function calcType(answers, gender) {
  const score = {}
  for (let i = 1; i <= 9; i++) score[i] = 0
  answers.forEach((opt) => {
    if (!opt) return
    Object.entries(opt.w).forEach(([id, v]) => { score[id] += v })
  })
  const gw = GENDER_WEIGHT[gender] || {}
  // 主分相同时，性别权重作为小数决胜（不改变明显领先者）
  const adjusted = {}
  for (let id = 1; id <= 9; id++) {
    adjusted[id] = score[id] + (score[id] * (gw[id] || 1) - score[id]) * 0.15
  }
  // 按加权分排序，得到主型 + 副型倾向
  const ranking = Object.keys(adjusted)
    .map((id) => ({ id: Number(id), raw: score[id], val: adjusted[id] }))
    .sort((a, b) => b.val - a.val)
  const best = ranking[0].id
  // 副型：分数最高且 > 0 的「非主型」
  const second = ranking.find((r) => r.id !== best && r.raw > 0)?.id || null

  // 三中心分布（按原始分占比）
  const centerScore = { gut: 0, heart: 0, head: 0 }
  for (let id = 1; id <= 9; id++) centerScore[TYPES_INFO[id].center] += score[id]
  const centerTotal = centerScore.gut + centerScore.heart + centerScore.head || 1
  const centers = ['gut', 'heart', 'head'].map((key) => ({
    key,
    name: CENTERS[key].name,
    pct: Math.round((centerScore[key] / centerTotal) * 100),
  }))

  return { type: best, second, score, ranking, centers }
}

// 主型与副型相邻（编号 ±1，9 与 1 相邻）即为「侧翼」，否则为「次要型」。
function isWing(main, other) {
  if (!other) return false
  const diff = Math.abs(main - other)
  return diff === 1 || diff === 8
}

export default function Game() {
  const [stage, setStage] = useState('intro') // intro | quiz | result
  const [gender, setGender] = useState(null)
  const [step, setStep] = useState(0)
  const [answers, setAnswers] = useState([])
  const reportedResultsRef = useRef(new Set())

  const result = useMemo(() => {
    if (stage !== 'result') return null
    return calcType(answers, gender)
  }, [stage, answers, gender])

  useEffect(() => {
    if (!result || !gender) return
    const reportKey = `${gender}|${result.type}|${result.second || 0}|${JSON.stringify(result.score)}`
    if (reportedResultsRef.current.has(reportKey)) return
    reportedResultsRef.current.add(reportKey)
    trackGameResult({
      centers: result.centers,
      gender,
      resultType: result.type,
      score: result.score,
      secondType: result.second || 0,
    })
  }, [gender, result])

  const start = (g) => { setGender(g); setStage('quiz'); setStep(0); setAnswers([]) }

  const choose = (opt) => {
    const next = [...answers]
    next[step] = opt
    setAnswers(next)
    if (step < QUESTIONS.length - 1) {
      setTimeout(() => setStep(step + 1), 180)
    } else {
      setTimeout(() => setStage('result'), 200)
    }
  }

  const restart = () => { setStage('intro'); setGender(null); setStep(0); setAnswers([]) }

  const [card, setCard] = useState({ open: false, url: '', loading: false })
  const [copied, setCopied] = useState('')
  const closeCard = () => { setCard({ open: false, url: '', loading: false }); setCopied('') }
  const makeShareCard = async () => {
    if (!result) return
    setCopied('')
    setCard({ open: true, url: '', loading: true })
    try {
      const url = await buildShareCard(result, RESULTS[result.type], TYPES_INFO[result.type])
      setCard({ open: true, url, loading: false })
    } catch {
      setCard({ open: false, url: '', loading: false })
    }
  }
  const copyCard = async () => {
    try {
      const blob = await (await fetch(card.url)).blob()
      await navigator.clipboard.write([new window.ClipboardItem({ 'image/png': blob })])
      setCopied('ok')
    } catch {
      setCopied('fail')
    }
    setTimeout(() => setCopied(''), 2000)
  }

  // ===== 开场 =====
  if (stage === 'intro') {
    return (
      <section className="game wrap">
        <div className="game__intro">
          <p className="eyebrow">趣味体验</p>
          <h1 className="display"><span className="gradient-text">你的人设出厂设置</span></h1>
          <p className="lead" style={{ margin: '16px auto 0', maxWidth: 560 }}>
            {QUESTIONS.length} 道情境小测，看看你天生自带的是哪一块「性格芯片」。先选择你的性别，让结果更贴近你。
          </p>
          <div className="game__gender">
            <button className="game__gender-card game__gender-card--m" onClick={() => start('male')}>
              <span className="game__gender-emoji">♂</span>
              <b>男生</b><small>开始测试</small>
            </button>
            <button className="game__gender-card game__gender-card--f" onClick={() => start('female')}>
              <span className="game__gender-emoji">♀</span>
              <b>女生</b><small>开始测试</small>
            </button>
          </div>
          <p className="game__tip">约 2 分钟 · 共 {QUESTIONS.length} 题 · 凭直觉选择最贴近你的那个</p>
        </div>
      </section>
    )
  }

  // ===== 答题 =====
  if (stage === 'quiz') {
    const q = QUESTIONS[step]
    const progress = ((step + (answers[step] ? 1 : 0)) / QUESTIONS.length) * 100
    return (
      <section className="game wrap">
        <div className="game__quiz">
          <div className="game__quizbar"><span style={{ width: `${progress}%` }} /></div>
          <div className="game__quizhead">
            <span>第 {step + 1} / {QUESTIONS.length} 题</span>
            {step > 0 && <button className="game__back" onClick={() => setStep(step - 1)}>← 上一题</button>}
          </div>
          <h2 className="game__q" key={step}>{q.q}</h2>
          <div className="game__options">
            {q.options.map((opt, k) => (
              <button
                key={k}
                className={`game__option ${answers[step] === opt ? 'on' : ''}`}
                style={{ '--d': `${k * 60}ms` }}
                onClick={() => choose(opt)}
              >
                <span className="game__option-idx">{String.fromCharCode(65 + k)}</span>
                {opt.t}
              </button>
            ))}
          </div>
        </div>
      </section>
    )
  }

  // ===== 结果 =====
  const r = RESULTS[result.type]
  const info = TYPES_INFO[result.type]
  const center = CENTERS[info.center]
  const persona = gender === 'male' ? r.male : r.female
  const secondInfo = result.second ? TYPES_INFO[result.second] : null
  const wing = isWing(result.type, result.second)
  const growthInfo = TYPES_INFO[info.growth]
  const stressInfo = TYPES_INFO[info.stress]

  return (
    <section className="game wrap">
      <div className="game__result">
        <Confetti />
        <p className="eyebrow" style={{ position: 'relative' }}>你的性格芯片</p>

        {/* GIF 欢庆区 */}
        <div className="result-gif">
          <span className="result-gif__ring result-gif__ring--1" />
          <span className="result-gif__ring result-gif__ring--2" />
          <span className="result-gif__glow" />
          <div className="result-gif__frame">
            <img
              src={`/assets/avatars/${result.type}.png`}
              alt={r.title}
              onError={(e) => { e.currentTarget.style.display = 'none'; e.currentTarget.nextElementSibling.style.display = 'grid' }}
            />
            <div className="result-gif__placeholder" style={{ display: 'none' }}>
              <span className="result-gif__num">{result.type}</span>
              <small>{info.name}</small>
            </div>
          </div>
          <span className="result-gif__badge">{result.type}</span>
        </div>

        <h1 className="result-title">{r.title}</h1>
        <p className="result-en">{info.en} · {info.keywords}</p>
        <p className="result-summary">{r.summary}</p>

        {/* 性别定制画像 */}
        <div className="result-persona">{persona}</div>

        {/* 核心驱动：基本恐惧 / 核心欲望 */}
        <div className="result-drive">
          <div className="result-drive__item result-drive__item--fear">
            <span className="result-drive__label">基本恐惧</span>
            <p>{info.fear}</p>
          </div>
          <div className="result-drive__item result-drive__item--desire">
            <span className="result-drive__label">核心欲望</span>
            <p>{info.desire}</p>
          </div>
        </div>

        <div className="result-grid">
          <div className="result-card">
            <h3>核心动机</h3>
            <p>{r.motive}</p>
          </div>
          <div className="result-card">
            <h3>所属中心</h3>
            <p><b>{center.name}</b><br />{center.desc}<br /><small>{center.issue}</small></p>
          </div>
          <div className="result-card">
            <h3>性格优势</h3>
            <ul>{r.strengths.map((s) => <li key={s}>{s}</li>)}</ul>
          </div>
          <div className="result-card">
            <h3>成长课题</h3>
            <ul>{r.challenges.map((s) => <li key={s}>{s}</li>)}</ul>
          </div>
        </div>

        {/* 副型倾向（基于你的答案） */}
        {secondInfo && (
          <div className="result-second">
            <h3>🧩 {wing ? '你的侧翼倾向' : '你的副型倾向'}</h3>
            <p>
              除了主型 <b>{result.type} 号 {info.name}</b>，你的答案里 <b>{result.second} 号 {secondInfo.name}</b> 的特质也很突出
              {wing
                ? `——它正好是你的侧翼（${result.type}w${result.second}），让你的「${info.name}」多了一层「${secondInfo.name}」的色彩。`
                : `——它会在不同情境下影响你的表现，让你比典型的「${info.name}」更立体。`}
            </p>
            <p className="result-second__kw">{secondInfo.keywords}</p>
          </div>
        )}

        {/* 侧翼说明 */}
        <div className="result-wings">
          <h3>🪶 两侧侧翼</h3>
          <div className="result-wings__row">
            {info.wings.map((w) => (
              <div key={w.id} className={`result-wing-card ${result.second === w.id ? 'on' : ''}`}>
                <b>{w.label.split(' ')[0]}</b>
                <small>{w.label.split(' ').slice(1).join(' ')}</small>
              </div>
            ))}
          </div>
        </div>

        {/* 三中心分布（基于你的答案） */}
        <div className="result-centers">
          <h3>📊 你的三中心分布</h3>
          {result.centers.map((c) => (
            <div key={c.key} className={`result-center-bar result-center-bar--${c.key}`}>
              <span className="result-center-bar__name">{c.name}</span>
              <span className="result-center-bar__track"><i style={{ width: `${c.pct}%` }} /></span>
              <span className="result-center-bar__pct">{c.pct}%</span>
            </div>
          ))}
          <p className="result-centers__tip">三中心反映你更多用「行动 / 情感 / 思考」哪条路径回应世界，占比越高代表那条路径越主导。</p>
        </div>

        {/* 动态箭头：压力 / 成长方向 */}
        <div className="result-arrows">
          <div className="result-arrow result-arrow--stress">
            <span className="result-arrow__tag">压力下 →</span>
            <b>{info.stress} 号 · {stressInfo.name}</b>
            <small>状态紧绷时，你可能滑向 {stressInfo.name} 的不健康面</small>
          </div>
          <div className="result-arrow result-arrow--growth">
            <span className="result-arrow__tag">成长时 →</span>
            <b>{info.growth} 号 · {growthInfo.name}</b>
            <small>状态健康时，你会活出 {growthInfo.name} 的优点</small>
          </div>
        </div>

        <div className="result-growth">
          <h3>💡 成长建议</h3>
          <p>{r.growth}</p>
        </div>

        <div className="btn-row" style={{ justifyContent: 'center', flexWrap: 'wrap' }}>
          <button className="btn btn--red" onClick={makeShareCard}>📸 保存分享卡片</button>
          <Link className="btn btn--blue" to="/#signup">预约一次深入解读 →</Link>
          <Link className="btn btn--ghost" to="/course">查看完整课件</Link>
          <button className="btn btn--ghost" onClick={restart}>重新测试</button>
        </div>
        <p className="result-disclaimer">本测试基于 The Enneagram Institute 九型人格体系简化设计，仅供趣味参考，不作专业诊断。真正的你，远比一个编号丰富。</p>
      </div>

      {card.open && (
        <div className="sharecard-mask" onClick={closeCard}>
          <div className="sharecard" onClick={(e) => e.stopPropagation()}>
            {card.loading ? (
              <p className="sharecard__loading">卡片生成中…</p>
            ) : (
              <>
                <img className="sharecard__img" src={card.url} alt="九型芯之力分享卡片" />
                <p className="sharecard__tip">
                  {copied === 'ok' ? '✅ 已复制，去粘贴吧' : copied === 'fail' ? '复制失败，请用「下载图片」' : '长按图片保存到相册，或下载 / 复制'}
                </p>
                <div className="sharecard__actions">
                  <a className="btn btn--red" href={card.url} download={`九型芯之力-${r.title}.png`}>下载图片</a>
                  <button className="btn btn--blue" onClick={copyCard}>复制图片</button>
                  <button className="btn btn--ghost" onClick={closeCard}>关闭</button>
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </section>
  )
}
