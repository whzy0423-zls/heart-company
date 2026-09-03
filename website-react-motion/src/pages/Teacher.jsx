import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

const ORGS = [
  '青岛迪安诊断集团', '罗氏集团（中国）', '安利（中国）日用品有限公司', '北京亿群投资控股',
  '清华大学环境设计学院', '北京广联达科技', '中国移动通信集团', '中国电力建设集团',
  '山东汇丰新材料科技', '天津欧陆宝新材料科技', '苏州云学堂网络科技', '山西神龙电缆等',
]

export default function Teacher() {
  return (
    <>
      {/* Hero */}
      <section className="hero wrap">
        <div className="hero__grid">
          <div>
            <p className="eyebrow">老师简介</p>
            <h1 className="display"><span className="gradient-text">韩常青<br />（老韩）</span></h1>
            <p className="lead" style={{ marginTop: 18 }}>北京九型成长平台、芯之力创始人。师从 Dr. Unico 陈伟志（陈博士）。陈博士是心理学及身心健康博士、九型人格学国际级导师、香港性格形态学会永久名誉会长，相关体系传承至今。老韩长期从事九型人格、教练技术、心理疏导治疗、系统排列、企业团队建设、亲子关系、夫妻关系与家庭关系培训。</p>
            <div className="btn-row">
              <a className="btn btn--red" href="/#signup">预约咨询</a>
              <a className="btn btn--ghost" href="/#courses">查看课程</a>
            </div>
          </div>
          <div style={{ display: 'grid', gap: 16, justifyItems: 'center' }}>
            <div className="poster-card" title="点击查看大图">
              <img src="/assets/teacher-poster.jpg" alt="老韩 · 看懂人，才能带好团队" data-zoom
                   onError={(e) => { e.currentTarget.closest('.poster-card').style.display = 'none' }} />
            </div>
            <p className="figcap">九型芯之力首席导师</p>
            <figure className="figure" style={{ maxWidth: 360 }}>
              <img src="/assets/teacher-mentor.jpg" alt="韩常青与陈伟志博士合影" loading="lazy" data-zoom
                   onError={(e) => { e.currentTarget.closest('figure').style.display = 'none' }} />
            </figure>
            <p className="figcap">与恩师 陈伟志博士（左）合影</p>
          </div>
        </div>
      </section>

      {/* 核心资历 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">核心资历</p>
          <h2 className="section-title">心理咨询、教练技术与家庭治疗背景</h2>
          <div className="grid grid-3" style={{ marginTop: 24 }}>
            <div className="card">国家二级心理咨询师</div>
            <div className="card">资深心理治疗师</div>
            <div className="card">NLP 教练技术资深教练</div>
            <div className="card">语言解码导师</div>
            <div className="card">系统排列导师</div>
            <div className="card">萨提亚家庭治疗师</div>
          </div>
        </Reveal>
      </section>

      {/* 时间线 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">学习与传承</p>
          <h2 className="section-title">二十多年一线导师经验</h2>
          <div className="timeline" style={{ marginTop: 26 }}>
            <div className="t-item"><div className="t-year">1999</div><p style={{ color: 'var(--ink-2)', fontSize: 14 }}>开始学习教练技术，持续进入个人成长与教练训练体系。</p></div>
            <div className="t-item"><div className="t-year">2004</div><p style={{ color: 'var(--ink-2)', fontSize: 14 }}>跟随香港性格学导师陈伟志博士学习九型系统，并持续传承至今。</p></div>
            <div className="t-item"><div className="t-year">2004–2009</div><p style={{ color: 'var(--ink-2)', fontSize: 14 }}>接受催眠与系统相关培训，持续体验、举办国际导师系统排列工作坊。</p></div>
            <div className="t-item"><div className="t-year">至今</div><p style={{ color: 'var(--ink-2)', fontSize: 14 }}>围绕个人、家庭、职场、组织与企业文化传承开展课程、咨询和团队训练。</p></div>
          </div>
        </Reveal>
      </section>

      {/* 擅长方向 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">擅长方向</p>
          <h2 className="section-title">把性格、情绪与关系放在一起疏通</h2>
          <div className="grid grid-3" style={{ marginTop: 24 }}>
            <div className="card"><h3>个人成长</h3><p style={{ color: 'var(--muted)', fontSize: 14, marginTop: 8 }}>通过性格与情绪能量识别，帮助个人理解自己的反应模式、关系模式和成长方向。</p></div>
            <div className="card"><h3>家庭关系</h3><p style={{ color: 'var(--muted)', fontSize: 14, marginTop: 8 }}>面向婚姻家庭、亲子、夫妻关系，提供咨询培训与一对一个案咨询治疗经验。</p></div>
            <div className="card"><h3>企业团队</h3><p style={{ color: 'var(--muted)', fontSize: 14, marginTop: 8 }}>服务企业团队建设、团队疏导、组织文化建设与管理沟通，提升协作与积极性。</p></div>
          </div>
        </Reveal>
      </section>

      {/* 课程方向 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">课程方向</p>
          <h2 className="section-title">九型、领导力、语言解码与家庭关系</h2>
          <ul className="bullets" style={{ marginTop: 20 }}>
            <li>《九型人格与生命关系》</li><li>《九型人格与领导力》</li><li>《企业团队训练》</li>
            <li>《语言解码》</li><li>《性格情绪 · 家庭亲子关系》</li>
          </ul>
        </Reveal>
      </section>

      {/* 服务经历 */}
      <section className="wrap block">
        <Reveal className="panel">
          <p className="eyebrow">服务经历</p>
          <h2 className="section-title">个人、家庭、职场与企业培训</h2>
          <p className="lead" style={{ marginTop: 12 }}>多年培训覆盖个人成长、家庭关系、职场协作、体制组织、企业团队与企业文化传承建设。</p>
          <div className="grid grid-3" style={{ marginTop: 22, gap: 14 }}>
            {ORGS.map((o) => (
              <div key={o} className="card" style={{ padding: '14px 18px' }}>{o}</div>
            ))}
          </div>
          <div className="btn-row">
            <a className="btn btn--red" href="/#courses">查看课程</a>
            <a className="btn btn--ghost" href="/#enterprise">企业工作坊</a>
          </div>
        </Reveal>
      </section>
    </>
  )
}
