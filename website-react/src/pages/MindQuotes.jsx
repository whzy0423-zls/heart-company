import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'
import { getMindGroups } from '../api/mindQuotes'

export default function MindQuotes() {
  const [groups, setGroups] = useState([])
  const [active, setActive] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    getMindGroups()
      .then((items) => {
        if (!alive) return
        setGroups(items)
        setLoading(false)
      })
      .catch((e) => {
        if (!alive) return
        setError(e.message || '加载失败')
        setLoading(false)
      })
    return () => {
      alive = false
    }
  }, [])

  const current = groups[active]

  return (
    <section className="wrap block mind-page">
      <div className="section-head">
        <p className="eyebrow">老韩语录</p>
        <h2 className="section-title">九型成长心语</h2>
        <p className="lead">
          从关系、觉察、学习、疗愈到能量与命运，按「脑 · 心 · 腹」三种能量中心整理老韩心语。选一句有感觉的话，点开读原文，停下来写一句自己的回应。
        </p>
      </div>

      {loading && <p className="mind-empty">心语加载中…</p>}
      {error && <p className="mind-empty">{error}</p>}

      {!loading && !error && groups.length === 0 && (
        <p className="mind-empty">心语内容即将上线</p>
      )}

      {!loading && !error && groups.length > 0 && (
        <>
          <div className="mind-tabs">
            {groups.map((g, i) => (
              <button
                key={g.id}
                className={`mind-tab${i === active ? ' on' : ''}`}
                onClick={() => setActive(i)}
              >
                {g.name}
                <span className="mind-tab__count">{g.quotes.length}</span>
              </button>
            ))}
          </div>

          {current && (
            <>
              {current.intro && <p className="mind-intro">{current.intro}</p>}
              {current.quotes.length === 0 ? (
                <p className="mind-empty">这个分组还没有心语</p>
              ) : (
                <div className="grid grid-3 mind-grid">
                  {current.quotes.map((q) => (
                    <Reveal
                      as={Link}
                      key={q.id}
                      to={`/mind-quotes/${q.id}`}
                      className="card mind-card"
                    >
                      <span className="mind-card__mark">”</span>
                      <p className="mind-card__text">{q.title}</p>
                      <span className="mind-card__more">读原文 →</span>
                    </Reveal>
                  ))}
                </div>
              )}
            </>
          )}
        </>
      )}
    </section>
  )
}
