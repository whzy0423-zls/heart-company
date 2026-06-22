import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { fetchArticles, fetchCategories } from '../api/articles.js'

const PAGE_SIZE = 10

function SearchIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
      strokeLinecap="round" strokeLinejoin="round">
      <circle cx="11" cy="11" r="7" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  )
}

function Cover({ article }) {
  if (article.cover) {
    return <img className="card-cover" src={article.cover} alt={article.title} loading="lazy" />
  }
  const ch = (article.title || '读').trim().charAt(0)
  return <div className="card-cover placeholder">{ch}</div>
}

export default function ListPage() {
  const navigate = useNavigate()
  const [keyword, setKeyword] = useState('')
  const [category, setCategory] = useState('')
  const [categories, setCategories] = useState([])
  const [items, setItems] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const debounceRef = useRef(null)

  useEffect(() => {
    fetchCategories().then((list) => setCategories(Array.isArray(list) ? list : [])).catch(() => {})
  }, [])

  const load = useCallback(async (nextPage, replace) => {
    setLoading(true)
    setError('')
    try {
      const res = await fetchArticles({ keyword, category, page: nextPage, pageSize: PAGE_SIZE })
      const list = res?.items || []
      setTotal(res?.total || 0)
      setItems((prev) => (replace ? list : [...prev, ...list]))
      setPage(nextPage)
    } catch (err) {
      setError(err.message || '加载失败')
    } finally {
      setLoading(false)
    }
  }, [keyword, category])

  // 关键词/分类变化时重新加载第一页（关键词做防抖）。
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => load(1, true), 300)
    return () => clearTimeout(debounceRef.current)
  }, [keyword, category, load])

  const hasMore = items.length < total

  return (
    <div className="shell">
      <header className="hero">
        <div className="hero-kicker">XINZHILI · READING</div>
        <h1 className="hero-title">芯之力 · 读书</h1>
        <p className="hero-sub">在文字里照见自己<br />把性格模式，慢慢转化为成长的力量</p>
      </header>

      <div className="toolbar">
        <div className="search">
          <SearchIcon />
          <input
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            placeholder="搜索文章标题或摘要"
            type="search"
          />
        </div>
        <div className="chips">
          <button
            className={`chip ${category === '' ? 'active' : ''}`}
            onClick={() => setCategory('')}
          >
            全部
          </button>
          {categories.map((cat) => (
            <button
              key={cat}
              className={`chip ${category === cat ? 'active' : ''}`}
              onClick={() => setCategory(cat)}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      {loading && items.length === 0 ? (
        <div className="state">
          <div className="spinner" />
          正在翻开书页…
        </div>
      ) : error ? (
        <div className="state">
          <div className="state-emoji">😕</div>
          {error}
        </div>
      ) : items.length === 0 ? (
        <div className="state">
          <div className="state-emoji">📭</div>
          还没有可阅读的文章
        </div>
      ) : (
        <>
          <div className="list">
            {items.map((article) => (
              <article
                key={article.id}
                className="card"
                onClick={() => navigate(`/article/${article.id}`)}
              >
                <Cover article={article} />
                <div className="card-body">
                  {article.category && <span className="card-cat">{article.category}</span>}
                  <h2 className="card-title">{article.title}</h2>
                  {article.summary && <p className="card-summary">{article.summary}</p>}
                  <div className="card-meta">
                    {article.author && <span>{article.author}</span>}
                    {article.author && <span className="dot" />}
                    <span>{article.publishTime?.slice(0, 10)}</span>
                    <span className="dot" />
                    <span>{article.viewCount || 0} 阅读</span>
                    {article.hasAudio && <span className="card-audio">🎧 听书</span>}
                  </div>
                </div>
              </article>
            ))}
          </div>

          {hasMore && (
            <button className="load-more" disabled={loading} onClick={() => load(page + 1, false)}>
              {loading ? '加载中…' : '加载更多'}
            </button>
          )}
        </>
      )}
    </div>
  )
}
