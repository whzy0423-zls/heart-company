import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'
import Wheel from '../components/Wheel'

export default function Stages() {
  return (
    <>
      {/* Hero */}
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">基础介绍</p>
            <h1 className="display"><span className="gradient-text">九型的三阶段</span></h1>
            <p className="lead" style={{ marginTop: 18 }}>九型是性格模式，也是性格散发出的能量气场。它影响一个人行为、做事、说话的方式，也是面对问题和解决问题的出发点。这份资料把九型的发展分为三个阶段：从苏菲灵修工具，到西方心理学分类，再到芯之力视角下的性格能量体系。</p>
            <div className="btn-row">
              <a className="btn btn--red" href="/#game">先体验小游戏</a>
              <a className="btn btn--ghost" href="/#signup">留下咨询需求</a>
            </div>
          </div>
          <Wheel />
        </div>
      </section>

      {/* 什么是九型 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">先理解九型</p>
          <h2 className="section-title">什么是九型</h2>
          <p className="lead" style={{ marginTop: 10 }}>九型不是给人贴标签，而是帮助我们看见稳定的性格模式、能量状态和行为出发点。</p>
          <div className="grid grid-3" style={{ marginTop: 24 }}>
            <div className="card course-card"><div className="card-head"><span className="badge">01</span><h3>性格模式</h3></div><p style={{ color: 'var(--muted)', fontSize: 14 }}>九型用来观察一个人稳定的行为、做事和说话方式。</p></div>
            <div className="card course-card"><div className="card-head"><span className="badge">02</span><h3>能量气场</h3></div><p style={{ color: 'var(--muted)', fontSize: 14 }}>性格会形成一种能量状态，影响面对问题和解决问题的出发点。</p></div>
            <div className="card course-card"><div className="card-head"><span className="badge">03</span><h3>芯片模式</h3></div><p style={{ color: 'var(--muted)', fontSize: 14 }}>有什么内在芯片，就会呈现相应的思维认知方式和行为模式。</p></div>
          </div>
        </Reveal>
      </section>

      {/* 演变三阶段 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">系统演变</p>
          <h2 className="section-title">九型人格演变的三阶段</h2>
          <p className="lead" style={{ marginTop: 10 }}>从苏菲灵修工具，到西方心理学分类，再到芯之力视角下的性格能量体系。</p>
          <div className="grid grid-3" style={{ marginTop: 24 }}>
            <Reveal className="card stage-card">
              <p className="kicker stage-card__kicker">第 01 阶段</p>
              <h3 className="stage-card__title">第一阶段：苏菲灵修期</h3>
              <p className="stage-card__sub">灵修起源，无能量概念</p>
              <div className="figure stage-card__media"><img src="/assets/stage1.svg" alt="" /></div>
              <p className="stage-card__body">公元 9 世纪苏菲教派把九型作为纯粹的灵修工具，口传心授，不分析性格、不讲能量。</p>
              <Link className="stage-card__link" to="/stage1">进入单独介绍 →</Link>
            </Reveal>
            <Reveal className="card stage-card">
              <p className="kicker stage-card__kicker">第 02 阶段</p>
              <h3 className="stage-card__title">第二阶段：西方心理学期</h3>
              <p className="stage-card__sub">行为分析，标签化性格</p>
              <div className="figure stage-card__media"><img src="/assets/types-map.svg" alt="" /></div>
              <p className="stage-card__body">上世纪六七十年代以后，九型被系统化用于心理学分析，从外在行为反推动机，形成测评与标签。</p>
              <Link className="stage-card__link" to="/stage2">进入单独介绍 →</Link>
            </Reveal>
            <Reveal className="card stage-card">
              <p className="kicker stage-card__kicker">第 03 阶段</p>
              <h3 className="stage-card__title">第三阶段：芯之力能量体系</h3>
              <p className="stage-card__sub">性格 = 能量呈现</p>
              <div className="figure stage-card__media stage3-media">
                <div className="stage3-media__logo">
                  <img src="/assets/wheel.png" alt="九型芯之力能量体系" />
                </div>
                <div className="stage3-media__mentor">
                  <img src="/assets/teacher-mentor.jpg" alt="韩常青与陈伟志博士合影" loading="lazy" />
                </div>
              </div>
              <p className="stage-card__body">芯之力视角提出性格是能量的呈现，是天生自带的生命芯片，强调看见天性、提升情商与能力。</p>
              <Link className="stage-card__link" to="/stage3">进入单独介绍 →</Link>
            </Reveal>
          </div>
        </Reveal>
      </section>

      {/* 对比表 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">阶段对比</p>
          <h2 className="section-title">三种理解方式有什么不同</h2>
          <div className="tbl-wrap" style={{ marginTop: 22 }}>
            <table className="tbl">
              <thead><tr><th>阶段</th><th>关注点</th><th>方式</th><th>特点</th></tr></thead>
              <tbody>
                <tr><td><b>苏菲灵修期</b></td><td>辨别天性、指引灵修</td><td>口传心授，不做性格标签</td><td>不讲能量概念</td></tr>
                <tr><td><b>西方心理学期</b></td><td>行为分析、动机分类</td><td>测评、分类、心理分析</td><td>容易标签化，未触及能量本质</td></tr>
                <tr><td><b>芯之力能量体系</b></td><td>性格本质、芯片能量</td><td>看见天性、接纳天性、提升能力</td><td>不评判好坏，重在成长应用</td></tr>
              </tbody>
            </table>
          </div>
        </Reveal>
      </section>

      {/* 芯之力视角 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">芯之力视角</p>
          <h2 className="section-title">芯之力视角：性格是能量，也是生命芯片</h2>
          <p className="lead" style={{ marginTop: 10 }}>性格不是简单的好坏评价，也不是固定标签。芯之力理论把性格看作内在芯片和能量气场的呈现：有什么芯片，就会有相应的行为模式、思维认知方式和能量状态。</p>
          <div className="grid grid-2" style={{ marginTop: 22 }}>
            <div className="card">性格不变，天生自带，真正要成长的是情商、能力与认知。</div>
            <div className="card">学习九型不是为了贴标签，而是理解自己和他人的性格能量状态。</div>
            <div className="card">能量视角保持中立，不评判好坏，只观察性格磁场和振频。</div>
            <div className="card">发现天性、接纳天性，才能把性格模式转化为更成熟的表达方式。</div>
          </div>
        </Reveal>
      </section>

      {/* 概述 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">三阶段概述</p>
          <h2 className="section-title">从灵修、心理分析到性格本质</h2>
          <ul className="bullets" style={{ marginTop: 20 }}>
            <li>灵修起源阶段重在辨别天性，不讲能量。</li>
            <li>心理分析阶段重在行为分类，容易形成标签。</li>
            <li>芯之力体系重在看见性格背后的能量本质。</li>
            <li>最终目标是理解自己、理解他人，并提升关系与行动能力。</li>
          </ul>
          <div className="btn-row"><a className="btn btn--red" href="/#stages">返回三阶段介绍</a></div>
        </Reveal>
      </section>
    </>
  )
}
