import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

export default function Stage3() {
  return (
    <>
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">第 03 阶段</p>
            <h1 className="display">第三阶段：<br /><span className="gradient-text">芯之力能量体系</span></h1>
            <p className="kicker" style={{ marginTop: 14 }}>性格 = 能量呈现</p>
            <p className="lead" style={{ marginTop: 12 }}>芯之力视角提出性格是能量的呈现，是天生自带的生命芯片。它强调看见天性、接纳天性，再提升情商、能力和认知。</p>
            <div className="steps">第 3 / 3 阶段</div>
            <div className="btn-row">
              <Link className="btn btn--ghost" to="/stages">返回三阶段总览</Link>
              <Link className="btn btn--ghost" to="/stage2">← 上一阶段</Link>
            </div>
          </div>
          <div style={{ display: 'grid', gap: 14, justifyItems: 'center' }}>
            <div className="figure" style={{ padding: 24, background: 'radial-gradient(circle at 50% 42%, #ffffff, #eef1f6)', display: 'grid', placeItems: 'center' }}><img src="/assets/wheel.png" alt="芯之力能量体系资料图" style={{ filter: 'drop-shadow(0 18px 32px rgba(28,40,70,.28))' }} /></div>
            <p className="figcap">九型芯之力资料图</p>
          </div>
        </div>
      </section>

      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">完整说明</p>
          <h2 className="section-title">这一阶段要看见什么</h2>
          <div className="grid grid-3" style={{ margin: '22px 0' }}>
            <div className="figure" style={{ padding: 12, background: '#fff' }}><img src="/assets/types-map.svg" alt="" /></div>
            <div className="figure" style={{ display: 'grid', placeItems: 'center', background: '#eceae4' }}><img src="/assets/logo.svg" alt="" style={{ maxWidth: '70%' }} /></div>
            <figure className="figure">
              <img src="/assets/teacher-mentor.jpg" alt="师承合影 · 陈伟志 与 韩常青" loading="lazy" data-zoom
                   onError={(e) => { e.currentTarget.closest('figure').style.display = 'none' }} />
            </figure>
          </div>
          <p className="figcap" style={{ marginTop: -8 }}>师承合影 · 陈伟志（左）与 韩常青</p>
          <ul className="bullets" style={{ marginTop: 18 }}>
            <li>1997 年后，芯之力理论提出性格是能量的呈现。</li>
            <li>性格不变，天生自带，性格就是生命芯片、内在芯片。</li>
            <li>有什么芯片，就有什么行为模式、思维认知方式和性格能量气场。</li>
            <li>能量视角保持中立：不评判好坏，不贴标签，只看性格能量状态、性格磁场和振频。</li>
            <li>真正可以成长的是情商、能力与认知。</li>
          </ul>
        </Reveal>
      </section>
    </>
  )
}
