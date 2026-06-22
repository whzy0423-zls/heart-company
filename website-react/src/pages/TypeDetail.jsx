import { Link, useParams } from 'react-router-dom'
import Reveal from '../components/Reveal'
import { TYPES_INFO, CENTERS, RESULTS } from '../data/enneagramGame'
import { TYPE_DETAILS, getTypeDetail } from '../data/typeDetails'

const TYPE_NAV = TYPE_DETAILS.map((item) => item.id)

export default function TypeDetail() {
  const { id } = useParams()
  const detail = getTypeDetail(id)
  const info = detail ? TYPES_INFO[detail.id] : null
  const result = detail ? RESULTS[detail.id] : null
  const center = info ? CENTERS[info.center] : null

  if (!detail || !info || !result) {
    return (
      <section className="wrap type-detail type-detail--empty">
        <Reveal className="panel">
          <p className="eyebrow">九型详情</p>
          <h1 className="section-title">没有找到这个型号</h1>
          <p className="lead" style={{ marginTop: 12 }}>可以回到九型概览，重新选择 1-9 号性格芯片。</p>
          <Link className="btn btn--blue" to="/#types" style={{ marginTop: 22 }}>回到九型概览</Link>
        </Reveal>
      </section>
    )
  }

  return (
    <section className="wrap type-detail">
      <div className="type-detail__nav" aria-label="九型型号切换">
        {TYPE_NAV.map((n) => (
          <Link key={n} className={n === detail.id ? 'active' : undefined} to={`/type/${n}`}>{n}</Link>
        ))}
      </div>

      <Reveal className={`type-hero type-hero--${info.color} panel`}>
        <div className="type-hero__copy">
          <p className="eyebrow">第 {detail.id} 型 · {info.en}</p>
          <h1 className="display"><span className="gradient-text">{info.name}</span></h1>
          <p className="lead">{result.summary}</p>
          <div className="type-hero__chips">
            <span>{info.keywords}</span>
            <span>{center.name}</span>
            <span>{detail.energy}</span>
          </div>
        </div>
        <div className="type-orbit" aria-hidden="true">
          <span className="type-orbit__ring type-orbit__ring--outer"></span>
          <span className="type-orbit__ring type-orbit__ring--inner"></span>
          <img src={`/assets/avatars/${detail.id}.png`} alt="" />
          <b>{detail.id}</b>
        </div>
      </Reveal>

      <div className="type-detail__grid">
        <Reveal className="card type-story">
          <p className="kicker">AI 生成介绍</p>
          <h2>{detail.scene}</h2>
          <p>{detail.intro}</p>
          <p>{detail.growth}</p>
        </Reveal>

        <Reveal className="card type-facts">
          <h2>核心动机</h2>
          <dl>
            <div>
              <dt>基本恐惧</dt>
              <dd>{info.fear}</dd>
            </div>
            <div>
              <dt>核心欲望</dt>
              <dd>{info.desire}</dd>
            </div>
            <div>
              <dt>成长方向</dt>
              <dd>{info.growth} 号 · {TYPES_INFO[info.growth].name}</dd>
            </div>
            <div>
              <dt>压力方向</dt>
              <dd>{info.stress} 号 · {TYPES_INFO[info.stress].name}</dd>
            </div>
          </dl>
        </Reveal>
      </div>

      <div className="type-suitability">
        <Reveal className="type-list card type-list--best">
          <p className="kicker">适合做什么</p>
          <h2>更容易发光的场景</h2>
          <ul>
            {detail.bestFor.map((item) => <li key={item}>{item}</li>)}
          </ul>
        </Reveal>

        <Reveal className="type-list card type-list--not">
          <p className="kicker">不适合做什么</p>
          <h2>需要谨慎选择的场景</h2>
          <ul>
            {detail.notFor.map((item) => <li key={item}>{item}</li>)}
          </ul>
        </Reveal>
      </div>

      <Reveal className="panel type-actions">
        <div>
          <p className="eyebrow">下一步</p>
          <h2 className="section-title">从了解型号，进入真实体验</h2>
          <p className="lead" style={{ marginTop: 12 }}>详情只能帮你看见倾向，测试和课程会帮你把这种倾向放回具体关系与行动里。</p>
        </div>
        <div className="btn-row">
          <Link className="btn btn--red" to="/game">测一测我的型号</Link>
          <Link className="btn btn--ghost" to="/course">滑动查看课件</Link>
        </div>
      </Reveal>
    </section>
  )
}
