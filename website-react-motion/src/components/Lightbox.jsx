import { useEffect, useState } from 'react'

// 全局图片预览灯箱：监听带 data-zoom 属性的 <img> 点击，弹出大图。
// 用事件委托，无需逐个图片绑定，新增图片只要加 data-zoom 即可。
export default function Lightbox() {
  const [src, setSrc] = useState(null)
  const [alt, setAlt] = useState('')

  useEffect(() => {
    const onClick = (e) => {
      const img = e.target.closest('img[data-zoom]')
      if (!img) return
      e.preventDefault()
      setSrc(img.currentSrc || img.src)
      setAlt(img.alt || '')
    }
    document.addEventListener('click', onClick)
    return () => document.removeEventListener('click', onClick)
  }, [])

  // 打开时锁定滚动 + 支持 ESC 关闭
  useEffect(() => {
    if (!src) return
    const onKey = (e) => { if (e.key === 'Escape') setSrc(null) }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    }
  }, [src])

  if (!src) return null
  return (
    <div className="lightbox" onClick={() => setSrc(null)}>
      <button className="lightbox__close" aria-label="关闭">×</button>
      <img className="lightbox__img" src={src} alt={alt} onClick={(e) => e.stopPropagation()} />
    </div>
  )
}
