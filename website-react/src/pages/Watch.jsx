import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

export default function Watch() {
  return (
    <section className="hero wrap">
      <div className="hero__grid">
        <div>
          <p className="eyebrow">开始观看</p>
          <h1 className="display"><span className="gradient-text">九型芯之力<br />观看入口</span></h1>
          <p className="lead" style={{ marginTop: 18 }}>这里先作为独立观看页，后面可以放老师介绍视频、课程试看视频或活动回放。</p>
          <div className="btn-row">
            <a className="btn btn--red" href="/#signup">注册/报名咨询</a>
            <Link className="btn btn--ghost" to="/stages">先看九型的三阶段</Link>
          </div>
        </div>
        <Reveal className="panel" style={{ aspectRatio: '4/3', display: 'grid', placeItems: 'center', textAlign: 'center' }}>
          <div>
            <div
              style={{ width: 140, height: 140, borderRadius: '50%', background: 'var(--blue)', color: '#fff', display: 'grid', placeItems: 'center', fontWeight: 800, fontSize: 20, margin: '0 auto 22px', boxShadow: 'var(--shadow)', cursor: 'pointer' }}
              onClick={(e) => { const el = e.currentTarget; el.style.transform = 'scale(.94)'; setTimeout(() => { el.style.transform = '' }, 150) }}
            >▶ 视频位置</div>
            <p style={{ fontWeight: 700, color: 'var(--ink-2)' }}>回头把视频放进来后，这里直接播放。</p>
          </div>
        </Reveal>
      </div>
    </section>
  )
}
