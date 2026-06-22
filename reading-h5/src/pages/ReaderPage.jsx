import { useEffect, useState, useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { marked } from 'marked'
import { fetchArticle, resolveMediaUrl } from '../api/articles.js'
import AudioPlayer from '../components/AudioPlayer.jsx'

marked.setOptions({ breaks: true, gfm: true })

function BackIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
      strokeLinecap="round" strokeLinejoin="round">
      <path d="m15 18-6-6 6-6" />
    </svg>
  )
}

export default function ReaderPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [article, setArticle] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let alive = true
    setLoading(true)
    setError('')
    window.scrollTo(0, 0)
    fetchArticle(id)
      .then((data) => { if (alive) setArticle(data) })
      .catch((err) => { if (alive) setError(err.message || '加载失败') })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [id])

  const html = useMemo(() => {
    if (!article?.content) return ''
    return marked.parse(article.content)
  }, [article])

  const audioUrl = useMemo(() => {
    if (article?.audioStatus === 'ready' && article?.audioUrl) {
      return resolveMediaUrl(article.audioUrl)
    }
    return ''
  }, [article])

  const goBack = () => {
    if (window.history.length > 1) navigate(-1)
    else navigate('/')
  }

  return (
    <div className="reader">
      <div className="reader-topbar">
        <button className="back-btn" onClick={goBack} aria-label="返回">
          <BackIcon />
        </button>
        <div className="reader-topbar-title">{article?.title || '阅读'}</div>
      </div>

      {loading ? (
        <div className="state">
          <div className="spinner" />
          正在加载文章…
        </div>
      ) : error ? (
        <div className="state">
          <div className="state-emoji">😕</div>
          {error}
        </div>
      ) : article ? (
        <article className="article">
          {article.category && <div className="article-cat">{article.category}</div>}
          <h1 className="article-title">{article.title}</h1>
          <div className="article-meta">
            {article.author && <span>{article.author}</span>}
            {article.author && <span className="dot" />}
            <span>{article.publishTime?.slice(0, 10)}</span>
            <span className="dot" />
            <span>{article.viewCount || 0} 阅读</span>
          </div>

          {audioUrl && <AudioPlayer src={audioUrl} title={article.title} />}

          {article.cover && (
            <img className="article-hero" src={article.cover} alt={article.title} />
          )}

          <div className="prose" dangerouslySetInnerHTML={{ __html: html }} />

          {article.tags?.length > 0 && (
            <div className="article-tags">
              {article.tags.map((tag) => (
                <span key={tag} className="article-tag">#{tag}</span>
              ))}
            </div>
          )}
        </article>
      ) : null}
    </div>
  )
}
