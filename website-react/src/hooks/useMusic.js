import { useEffect, useRef, useState } from 'react'

// 背景氛围音（WebAudio 生成式治愈系，无需音频文件）
// 五声音阶随机轻奏 + 低频 pad + 混响，安静温暖，契合成长/疗愈类官网气质
export function useMusic() {
  const [playing, setPlaying] = useState(false)
  const [volume, setVolume] = useState(50)
  const ref = useRef({ ctx: null, master: null, lp: null, pad: null, timer: null, playing: false })

  // C 大调五声音阶（C D E G A，跨两个八度）—— 任意组合都和谐，不会刺耳
  const SCALE = [261.63, 293.66, 329.63, 392.0, 440.0, 523.25, 587.33, 659.25, 783.99]

  function makeReverb(ctx) {
    const len = Math.floor(ctx.sampleRate * 2.8)
    const buf = ctx.createBuffer(2, len, ctx.sampleRate)
    for (let ch = 0; ch < 2; ch++) {
      const d = buf.getChannelData(ch)
      for (let i = 0; i < len; i++) d[i] = (Math.random() * 2 - 1) * Math.pow(1 - i / len, 2.6)
    }
    const c = ctx.createConvolver()
    c.buffer = buf
    return c
  }

  function note(s, freq, t, dur, peak) {
    const { ctx, lp } = s
    const g = ctx.createGain()
    g.gain.setValueAtTime(0.0001, t)
    g.gain.exponentialRampToValueAtTime(peak, t + 0.12)
    g.gain.exponentialRampToValueAtTime(0.0001, t + dur)
    g.connect(lp)
    ;[['sine', 0, 1], ['triangle', 5, 0.3]].forEach(([type, det, mix]) => {
      const o = ctx.createOscillator()
      o.type = type
      o.frequency.value = freq
      o.detune.value = det
      const og = ctx.createGain()
      og.gain.value = mix
      o.connect(og)
      og.connect(g)
      o.start(t)
      o.stop(t + dur + 0.2)
    })
  }

  function schedule() {
    const s = ref.current
    if (!s.playing) return
    const t = s.ctx.currentTime + 0.05
    const i = Math.floor(Math.random() * SCALE.length)
    note(s, SCALE[i], t, 3.4 + Math.random() * 1.6, 0.16)
    if (Math.random() < 0.4) {
      const j = Math.min(SCALE.length - 1, i + 2 + (Math.random() < 0.5 ? 1 : 0))
      note(s, SCALE[j], t + 0.18, 3.0 + Math.random() * 1.4, 0.1)
    }
    s.timer = setTimeout(schedule, 1600 + Math.random() * 1800)
  }

  function start() {
    const s = ref.current
    const ctx = s.ctx || new (window.AudioContext || window.webkitAudioContext)()
    s.ctx = ctx
    if (ctx.state === 'suspended') ctx.resume()

    const master = ctx.createGain()
    master.gain.value = 0
    master.gain.linearRampToValueAtTime((volume / 100) * 0.7, ctx.currentTime + 1.2)
    master.connect(ctx.destination)

    const lp = ctx.createBiquadFilter()
    lp.type = 'lowpass'
    lp.frequency.value = 1900
    lp.Q.value = 0.6
    lp.connect(master) // 干声

    const rev = makeReverb(ctx) // 湿声（空间感）
    const revGain = ctx.createGain()
    revGain.gain.value = 0.55
    lp.connect(rev)
    rev.connect(revGain)
    revGain.connect(master)

    // 低频铺底 pad：根音 + 五度，极轻，随 LFO 缓慢起伏
    const padG = ctx.createGain()
    padG.gain.value = 0.05
    padG.connect(lp)
    const lfo = ctx.createOscillator()
    const lfoG = ctx.createGain()
    lfo.frequency.value = 0.07
    lfoG.gain.value = 0.025
    lfo.connect(lfoG)
    lfoG.connect(padG.gain)
    lfo.start()
    const pads = [130.81, 196.0].map((f) => {
      const o = ctx.createOscillator()
      o.type = 'sine'
      o.frequency.value = f
      o.connect(padG)
      o.start()
      return o
    })

    s.master = master
    s.lp = lp
    s.pad = [...pads, lfo]
    s.playing = true
    schedule()
  }

  function stop() {
    const s = ref.current
    s.playing = false
    if (s.timer) {
      clearTimeout(s.timer)
      s.timer = null
    }
    if (s.master) s.master.gain.linearRampToValueAtTime(0.0001, s.ctx.currentTime + 0.6)
    const dead = s.pad
    s.pad = null
    setTimeout(() => (dead || []).forEach((n) => {
      try { n.stop() } catch (e) { /* noop */ }
    }), 700)
  }

  function toggle() {
    setPlaying((p) => {
      const next = !p
      next ? start() : stop()
      return next
    })
  }

  // 音量实时生效
  useEffect(() => {
    const s = ref.current
    if (s.master && s.ctx) s.master.gain.setTargetAtTime((volume / 100) * 0.7, s.ctx.currentTime, 0.1)
  }, [volume])

  // 卸载时清理
  useEffect(() => () => {
    const s = ref.current
    if (s.timer) clearTimeout(s.timer)
    ;(s.pad || []).forEach((n) => {
      try { n.stop() } catch (e) { /* noop */ }
    })
  }, [])

  return { playing, toggle, volume, setVolume }
}
