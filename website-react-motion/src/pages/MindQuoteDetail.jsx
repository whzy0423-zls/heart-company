import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import Reveal from '../components/Reveal'
import { getMindQuote } from '../api/mindQuotes'

export default function MindQuoteDetail() {
  const { id } = useParams()
  const [quote, setQuote] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    setLoading(true)
    getMindQuote(id)
      .then((data) => {
        if (!alive) return
        setQuote(data)
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
  }, [id])

  if (loading) {
    return (
      <section className="wrap block mind-detail">
        <p className="mind-empty">心语加载中…</p>
      </section>
    )
  }

  if (error || !quote) {
    return (
      <section className="wrap block mind-detail">
        <Reveal className="panel">
          <p className="eyebrow">成长心语</p>
          <h1 className="section-title">没有找到这条心语</h1>
          <p className="lead" style={{ marginTop: 12 }}>它可能已下架，回到心语墙再选一句吧。</p>
          <Link className="btn btn--blue" to="/mind-quotes" style={{ marginTop: 22 }}>回到成长心语</Link>
        </Reveal>
      </section>
    )
  }

  // 原文按换行拆段渲染
  const paragraphs = (quote.content || '').split('\n').filter((s) => s.trim())

  return (
    <section className="wrap block mind-detail">
      <Reveal className="mind-detail__card panel">
        <p className="eyebrow">老韩语录 · 九型成长心语</p>
        <h1 className="mind-detail__title">{quote.title}</h1>
        <div className="mind-detail__body">
          {paragraphs.map((p, i) => (
            <p key={i}>{p}</p>
          ))}
        </div>
        {quote.prompt && (
          <div className="mind-detail__prompt">
            <span className="mind-detail__prompt-label">回应提示</span>
            <p>{quote.prompt}</p>
          </div>
        )}
        <div className="mind-detail__actions">
          <Link className="btn btn--blue" to="/mind-quotes">← 回到成长心语</Link>
          <Link className="btn btn--red" to="/signup">预约一次深入解读</Link>
        </div>
      </Reveal>
    </section>
  )
}
