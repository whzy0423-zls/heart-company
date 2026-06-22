import { useEffect, useRef, useState } from 'react'

function PlayIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M8 5v14l11-7z" />
    </svg>
  )
}

function PauseIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor">
      <path d="M6 5h4v14H6zM14 5h4v14h-4z" />
    </svg>
  )
}

function fmt(sec) {
  if (!Number.isFinite(sec) || sec < 0) return '0:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  return `${m}:${String(s).padStart(2, '0')}`
}

const SPEEDS = [1, 1.25, 1.5, 2]

// 悬浮听书播放器：原生 audio，纸张风格 UI。
export default function AudioPlayer({ src, title }) {
  const audioRef = useRef(null)
  const [playing, setPlaying] = useState(false)
  const [current, setCurrent] = useState(0)
  const [duration, setDuration] = useState(0)
  const [speed, setSpeed] = useState(1)
  const [ready, setReady] = useState(false)

  useEffect(() => {
    // 切换文章时重置播放状态。
    setPlaying(false)
    setCurrent(0)
    setDuration(0)
    setReady(false)
  }, [src])

  useEffect(() => {
    if (audioRef.current) audioRef.current.playbackRate = speed
  }, [speed])

  const toggle = () => {
    const el = audioRef.current
    if (!el) return
    if (playing) {
      el.pause()
    } else {
      el.play().catch(() => {})
    }
  }

  const onSeek = (e) => {
    const el = audioRef.current
    if (!el || !duration) return
    const value = Number(e.target.value)
    el.currentTime = value
    setCurrent(value)
  }

  const cycleSpeed = () => {
    const next = SPEEDS[(SPEEDS.indexOf(speed) + 1) % SPEEDS.length]
    setSpeed(next)
  }

  const progress = duration ? (current / duration) * 100 : 0

  return (
    <div className="audio-player">
      <audio
        ref={audioRef}
        src={src}
        preload="metadata"
        onLoadedMetadata={(e) => {
          setDuration(e.currentTarget.duration || 0)
          setReady(true)
          e.currentTarget.playbackRate = speed
        }}
        onTimeUpdate={(e) => setCurrent(e.currentTarget.currentTime)}
        onPlay={() => setPlaying(true)}
        onPause={() => setPlaying(false)}
        onEnded={() => {
          setPlaying(false)
          setCurrent(0)
        }}
      />
      <button
        className="audio-toggle"
        onClick={toggle}
        disabled={!ready}
        aria-label={playing ? '暂停' : '播放'}
      >
        {playing ? <PauseIcon /> : <PlayIcon />}
      </button>
      <div className="audio-body">
        <div className="audio-title">🎧 {playing ? '正在朗读' : '听本文'}{title ? ` · ${title}` : ''}</div>
        <input
          className="audio-range"
          type="range"
          min={0}
          max={duration || 0}
          step="0.1"
          value={current}
          onChange={onSeek}
          style={{ '--progress': `${progress}%` }}
        />
        <div className="audio-meta">
          <span>{fmt(current)}</span>
          <span>{fmt(duration)}</span>
        </div>
      </div>
      <button className="audio-speed" onClick={cycleSpeed}>
        {speed}×
      </button>
    </div>
  )
}
