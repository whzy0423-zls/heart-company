import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'
import { VIDEOS } from '../data/videos'

export default function Watch() {
  const playerRef = useRef(null)
  const didSelectVideo = useRef(false)
  const [activeId, setActiveId] = useState(VIDEOS[0].id)
  const active = VIDEOS.find((video) => video.id === activeId) || VIDEOS[0]

  useEffect(() => {
    if (!didSelectVideo.current) return
    const player = playerRef.current
    if (!player) return
    player.currentTime = 0
    player.play().catch(() => {})
  }, [activeId])

  const selectVideo = (id) => {
    didSelectVideo.current = true
    setActiveId(id)
  }

  const pauseBackgroundMusic = () => {
    window.dispatchEvent(new CustomEvent('site:pause-music'))
  }

  return (
    <section className="watch wrap">
      <div className="watch__head">
        <Reveal>
          <p className="eyebrow">开始观看</p>
          <h1 className="display"><span className="gradient-text">九型芯之力<br />观看入口</span></h1>
          <p className="lead" style={{ marginTop: 18 }}>课程试看、老师介绍和活动回放统一放在这里。首页展示精选片段，完整视频列表可以在这里继续观看。</p>
          <div className="btn-row">
            <a className="btn btn--red" href="/#signup">注册/报名咨询</a>
            <Link className="btn btn--ghost" to="/stages">先看九型的三阶段</Link>
          </div>
        </Reveal>
      </div>

      <div className="watch__grid">
        <div className="watch__player-column">
          <Reveal className="watch-player panel">
            <div className="watch-player__screen">
              <video ref={playerRef} key={active.id} controls preload="metadata" poster={active.poster} onPlay={pauseBackgroundMusic}>
                <source src={active.src} type="video/mp4" />
              </video>
            </div>
            <div className="watch-player__meta">
              <span>{active.tag}</span>
              <span>{active.duration}</span>
            </div>
            <h2>{active.title}</h2>
            <p>{active.description}</p>
          </Reveal>
        </div>

        <div className="watch__list-column">
          <Reveal className="video-list card">
            <div className="video-list__head">
              <div>
                <p className="kicker">全部视频</p>
                <h2>选择想看的片段</h2>
              </div>
              <span>{VIDEOS.length} 个视频</span>
            </div>
            <div className="video-list__grid">
              {VIDEOS.map((video) => (
                <button
                  key={video.id}
                  className={video.id === active.id ? 'video-item active' : 'video-item'}
                  onClick={() => selectVideo(video.id)}
                >
                  <img src={video.poster} alt="" loading="lazy" />
                  <span>
                    <b>{video.title}</b>
                    <small>{video.tag} · {video.duration}</small>
                  </span>
                </button>
              ))}
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  )
}
