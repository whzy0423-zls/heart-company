import { useRef, useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { COURSE } from '../data/course'

export default function Course() {
  const slides = COURSE.slides
  const total = slides.length
  const [i, setI] = useState(0)
  const [tocOpen, setTocOpen] = useState(false)
  const sliderRef = useRef(null)

  const go = useCallback((n) => {
    const nextIndex = Math.min(total - 1, Math.max(0, n))
    const slider = sliderRef.current
    if (slider) {
      slider.scrollTo({ left: nextIndex * slider.clientWidth, behavior: 'smooth' })
    }
    setI(nextIndex)
  }, [total])

  useEffect(() => {
    const onKey = (e) => {
      if (e.key === 'ArrowRight') go(i + 1)
      else if (e.key === 'ArrowLeft') go(i - 1)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [go, i])

  const progress = ((i + 1) / total) * 100

  const handleSlideScroll = useCallback((e) => {
    const el = e.currentTarget
    if (!el.clientWidth) return
    const nextIndex = Math.round(el.scrollLeft / el.clientWidth)
    setI(Math.min(total - 1, Math.max(0, nextIndex)))
  }, [total])

  // 跳到某章节的分隔页
  const jumpToChapter = (ch) => {
    const idx = slides.findIndex((sl) => sl.kind === 'divider' && sl.ch === ch)
    if (idx >= 0) go(idx)
    setTocOpen(false)
  }

  return (
    <section className="course wrap">
      <div className="course__top">
        <div>
          <p className="eyebrow">系统课件</p>
          <h1 className="course__maintitle">{COURSE.title}<span>· {COURSE.subtitle}</span></h1>
        </div>
        <div className="course__top-actions">
          <button className="btn btn--ghost course__tocbtn" onClick={() => setTocOpen(true)}>目录</button>
          <Link className="btn btn--ghost" to="/stages">三阶段详解</Link>
        </div>
      </div>

      {/* 进度条 */}
      <div className="course__bar"><span style={{ width: `${progress}%` }} /></div>

      {/* 滑动观看舞台 */}
      <div className="course__stage course__stage--swipe">
        <div className="course__slider" ref={sliderRef} onScroll={handleSlideScroll} aria-label="滑动浏览课件">
          {slides.map((s, index) => (
            <article className={`slide slide--${s.kind}`} key={`${s.kind}-${index}`}>
              {s.kind === 'cover' && (
                <div className="slide__cover">
                  <img className="slide__cover-logo" src="/assets/wheel.png" alt="" />
                  <h2>{s.title}</h2>
                  <p className="slide__cover-sub">{s.subtitle}</p>
                  <p className="slide__cover-note">{s.note}</p>
                </div>
              )}

              {s.kind === 'divider' && (
                <div className="slide__divider">
                  <span className="slide__divider-no">{s.no}</span>
                  <h2>{s.title}</h2>
                  {s.sub && <p>{s.sub}</p>}
                </div>
              )}

              {s.kind === 'content' && (
                <div className="slide__content">
                  <span className="slide__chapter">{COURSE.chapters[s.ch]}</span>
                  <h2 className="slide__title">{s.title}</h2>
                  <div className="slide__items">
                    {s.items.map((it, k) => (
                      <div className="slide__item" key={k} style={{ '--d': `${k * 90}ms` }}>
                        <span className="slide__item-no">{String(k + 1).padStart(2, '0')}</span>
                        <div>
                          <h3>{it.h}</h3>
                          <p>{it.t}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {s.kind === 'end' && (
                <div className="slide__cover slide__end">
                  <h2>{s.title}</h2>
                  <p className="slide__cover-note">{s.note}</p>
                  <Link className="btn btn--red" to="/#signup" style={{ marginTop: 18 }}>预约咨询课程</Link>
                </div>
              )}
            </article>
          ))}
        </div>
        <p className="course__hint">左右滑动，自行观看课件</p>
      </div>

      {/* 底部页码 + 点导航 */}
      <div className="course__foot">
        <span className="course__page">{i + 1} / {total}</span>
        <div className="course__dots">
          {slides.map((_, k) => (
            <button key={k} className={k === i ? 'on' : undefined} onClick={() => go(k)} aria-label={`第${k + 1}页`} />
          ))}
        </div>
      </div>

      {/* 目录抽屉 */}
      <div className={tocOpen ? 'course__toc open' : 'course__toc'} onClick={() => setTocOpen(false)}>
        <div className="course__toc-panel" onClick={(e) => e.stopPropagation()}>
          <div className="course__toc-head">
            <h3>课程目录</h3>
            <button onClick={() => setTocOpen(false)} aria-label="关闭">×</button>
          </div>
          {COURSE.chapters.map((c, ch) => (
            <button key={ch} className="course__toc-item" onClick={() => jumpToChapter(ch)}>
              <span>{String(ch + 1).padStart(2, '0')}</span>{c}
            </button>
          ))}
        </div>
      </div>
    </section>
  )
}
