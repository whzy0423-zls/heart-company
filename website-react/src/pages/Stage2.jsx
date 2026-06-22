import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

export default function Stage2() {
  return (
    <>
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">第 02 阶段</p>
            <h1 className="display">第二阶段：<br /><span className="gradient-text">西方心理学期</span></h1>
            <p className="kicker" style={{ marginTop: 14 }}>行为分析，标签化性格</p>
            <p className="lead" style={{ marginTop: 12 }}>上世纪六七十年代以后，九型被系统化并用于心理学分析，通过外在行为反推性格动机，形成分类、测评和标签，但仍未触及性格能量本质。</p>
            <div className="steps">第 2 / 3 阶段</div>
            <div className="btn-row">
              <Link className="btn btn--ghost" to="/stages">返回三阶段总览</Link>
              <Link className="btn btn--ghost" to="/stage1">← 上一阶段</Link>
              <Link className="btn btn--red" to="/stage3">下一阶段 →</Link>
            </div>
          </div>
          <div style={{ display: 'grid', gap: 14, justifyItems: 'center' }}>
            <div className="figure"><img src="/assets/types-map.svg" alt="九型人格性格类型图" /></div>
            <p className="figcap">九型芯之力资料图</p>
          </div>
        </div>
      </section>

      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">完整说明</p>
          <h2 className="section-title">这一阶段要看见什么</h2>
          <div className="figure reveal" style={{ maxWidth: 420, margin: '22px 0' }}><img src="/assets/types-map.svg" alt="" /></div>
          <ul className="bullets" style={{ marginTop: 6 }}>
            <li>上世纪六七十年代，九型被系统化、心理学化应用。</li>
            <li>核心方式是从外在行为反推性格动机。</li>
            <li>这个阶段形成分类、测评和标签，也容易产生好坏评判。</li>
            <li>特点是有性格分析，但仍未触及性格能量本质。</li>
          </ul>
        </Reveal>
      </section>
    </>
  )
}
