import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

export default function Stage1() {
  return (
    <>
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">第 01 阶段</p>
            <h1 className="display">第一阶段：<br /><span className="gradient-text">苏菲灵修期</span></h1>
            <p className="kicker" style={{ marginTop: 14 }}>灵修起源，无能量概念</p>
            <p className="lead" style={{ marginTop: 12 }}>公元 9 世纪苏菲教派把九型作为纯粹的灵修工具，口传心授，主要用于辨别弟子天性、指引灵修路径，不分析性格、不讲能量。</p>
            <div className="steps">第 1 / 3 阶段</div>
            <div className="btn-row">
              <Link className="btn btn--ghost" to="/stages">返回三阶段总览</Link>
              <Link className="btn btn--red" to="/stage2">下一阶段 →</Link>
            </div>
          </div>
          <div style={{ display: 'grid', gap: 14, justifyItems: 'center' }}>
            <div className="figure"><img src="/assets/stage1.svg" alt="苏菲灵修期资料图" /></div>
            <p className="figcap">九型芯之力资料图</p>
          </div>
        </div>
      </section>

      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">完整说明</p>
          <h2 className="section-title">这一阶段要看见什么</h2>
          <div className="figure reveal" style={{ maxWidth: 420, margin: '22px 0' }}><img src="/assets/stage1.svg" alt="" /></div>
          <ul className="bullets" style={{ marginTop: 6 }}>
            <li>公元 9 世纪苏菲教派，把九型作为纯粹的灵修工具。</li>
            <li>通过口传心授辨别弟子天性，指引灵修路径。</li>
            <li>这个阶段不分析性格，不做标签，也不讲能量学说。</li>
            <li>关键词：灵修开始、口传、无文字、无评判、无能量概念。</li>
          </ul>
        </Reveal>
      </section>
    </>
  )
}
