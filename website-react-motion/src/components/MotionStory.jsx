import { Link } from 'react-router-dom'

const steps = [
  ['01', '看见模式', '识别反复出现的情绪、行为和关系反应。'],
  ['02', '理解动力', '越过性格标签，看见每种反应背后的真实需要。'],
  ['03', '转化能量', '接纳天性，扩展选择，把性格变成成长的力量。'],
]

export default function MotionStory() {
  return (
    <section className="motion-story" data-motion-section="growth-story" data-pointer-spotlight>
      <div className="motion-story__spotlight" aria-hidden="true" />
      <div className="motion-story__signal" aria-hidden="true">
        {Array.from({ length: 9 }, (_, index) => <span key={index} style={{ '--signal-index': index }} />)}
      </div>
      <div className="motion-story__kinetic" aria-hidden="true">
        <p data-kinetic-track>看见自己 · 理解他人 · 转化能量 · INNER POWER ·</p>
      </div>
      <div className="wrap motion-story__inner">
        <div className="motion-story__intro motion-reveal" data-motion-reveal>
          <p className="motion-kicker">THE PATH OF INNER POWER</p>
          <h2 className="motion-story__title" data-motion-title>
            <span className="motion-word">成长，</span>
            <span className="motion-word">不是改变成别人，</span>
            <span className="motion-word motion-word--accent">而是看清自己。</span>
          </h2>
        </div>
        <div className="motion-story__steps">
          {steps.map(([number, title, copy]) => (
            <article className="motion-story__step motion-reveal" data-motion-reveal key={number}>
              <span>{number}</span>
              <div><h3>{title}</h3><p>{copy}</p></div>
            </article>
          ))}
        </div>
        <Link className="motion-story__link" to="/stages" data-magnetic>进入九型的三阶段 <span aria-hidden="true">↗</span></Link>
      </div>
    </section>
  )
}
