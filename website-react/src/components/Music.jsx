import { useMusic } from '../hooks/useMusic'

// 浮动音乐控件（UI 与原站一致，逻辑走 useMusic 治愈系氛围音）
export default function Music() {
  const { playing, toggle, volume, setVolume } = useMusic()
  return (
    <div className={playing ? 'music playing' : 'music'}>
      <button onClick={toggle}>
        <span className="bars"><span></span><span></span><span></span></span>
        <span className="label">{playing ? '关闭音乐' : '开启音乐'}</span>
      </button>
      <label>音量</label>
      <input
        type="range"
        min="0"
        max="100"
        value={volume}
        onChange={(e) => setVolume(+e.target.value)}
      />
    </div>
  )
}
