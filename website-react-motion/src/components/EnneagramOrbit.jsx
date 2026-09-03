import { Link } from 'react-router-dom'

const TYPES = [
  ['9', '和平型', 'green'], ['1', '完美型', 'blue'], ['2', '助人型', 'green'],
  ['3', '成就型', 'coral'], ['4', '自我型', 'blue'], ['5', '观察型', 'green'],
  ['6', '忠诚型', 'blue'], ['7', '活跃型', 'coral'], ['8', '领袖型', 'coral'],
]

export default function EnneagramOrbit() {
  return (
    <div className="motion-orbit-wrap" data-motion-section="enneagram-orbit">
      <div className="motion-orbit" aria-label="九型人格互动轨道">
        <span className="motion-orbit__halo" />
        <span className="motion-orbit__ring motion-orbit__ring--outer" />
        <span className="motion-orbit__ring motion-orbit__ring--inner" />
        <div className="motion-orbit__core">
          <img src="/assets/wheel.png" alt="九型芯之力" />
        </div>
        {TYPES.map(([number, name, tone], index) => (
          <Link
            key={number}
            className={`motion-orbit__node motion-orbit__node--${tone}`}
            to={`/type/${number}`}
            style={{ '--angle': `${index * 40}deg` }}
            aria-label={`查看${number}号${name}详情`}
          >
            <span><b>{number}</b><span>{name}</span></span>
          </Link>
        ))}
        <span className="motion-orbit__caption">9 patterns · 1 complete self</span>
      </div>
    </div>
  )
}
