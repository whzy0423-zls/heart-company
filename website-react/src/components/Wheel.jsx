// 首页 / 三阶段页共用的九型环：中央抠图圆盘 + 九型节点环绕 + 光环特效
const WHEEL = [
  { n: '9', name: '和平型', c: 'blue',  a: 0 },
  { n: '1', name: '完美型', c: 'green', a: 40 },
  { n: '2', name: '助人型', c: 'blue',  a: 80 },
  { n: '3', name: '成就型', c: 'red',   a: 120 },
  { n: '4', name: '自我型', c: 'blue',  a: 160 },
  { n: '5', name: '观察型', c: 'green', a: 200 },
  { n: '6', name: '忠诚型', c: 'green', a: 240 },
  { n: '7', name: '活跃型', c: 'red',   a: 280 },
  { n: '8', name: '领袖型', c: 'red',   a: 320 },
]

export default function Wheel({ caption = '性格模式 · 能量气场 · 芯片模式' }) {
  return (
    <div className="hero__art" style={{ display: 'grid', placeItems: 'center' }}>
      <div className="wheel">
        <span className="wheel__ring wheel__ring--1" aria-hidden="true"></span>
        <span className="wheel__ring wheel__ring--2" aria-hidden="true"></span>
        <div className="orbit">
          <img className="chip-logo chip-logo--photo" src="/assets/wheel.png" alt="九型芯片" />
        </div>
        {WHEEL.map((t, i) => (
          <div
            key={t.n}
            className={`wheel__node wheel__node--${t.c}`}
            style={{ '--a': `${t.a}deg`, '--i': i }}
          >
            <span className="wheel__node-inner">
              <b>{t.n}</b>
              <span>{t.name}</span>
            </span>
          </div>
        ))}
      </div>
      {caption && <p className="kicker" style={{ marginTop: 18 }}>{caption}</p>}
    </div>
  )
}
