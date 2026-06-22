import Reveal from '../components/Reveal'
import siteConfig from '../data/siteConfig'

export default function Courses() {
  const courses = siteConfig.home.courses

  return (
    <section className="wrap block">
      <Reveal className="section-head">
        <p className="eyebrow">{courses.eyebrow}</p>
        <h1 className="display" style={{ fontSize: 'clamp(42px, 5vw, 68px)' }}>{courses.title}</h1>
      </Reveal>
      <div className="grid grid-3">
        {courses.items.map((course) => (
          <Reveal className="card course-card" key={course.badge}>
            <div className="card-head">
              <span className="badge">{course.badge}</span>
              <h3>{course.title}</h3>
            </div>
            <p style={{ color: 'var(--muted)', fontSize: '14.5px' }}>{course.description}</p>
            <ul className="bullets" style={{ marginTop: 14, fontSize: 14 }}>
              {course.bullets.map((item) => <li key={item}>{item}</li>)}
            </ul>
          </Reveal>
        ))}
      </div>
    </section>
  )
}
