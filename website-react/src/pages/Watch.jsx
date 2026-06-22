import { useState } from 'react'
import { Link } from 'react-router-dom'
import Reveal from '../components/Reveal'

const VIDEOS = [
  {
    id: 'intro',
    title: '九型芯之力 · 入门导览',
    duration: '08:32',
    tag: '基础介绍',
    description: '从“性格芯片”的角度，快速理解九型不是标签，而是一套观察内在动力的语言。',
    poster: '/assets/wheel-original.jpg',
    src: '',
  },
  {
    id: 'teacher',
    title: '老韩老师 · 课程试看',
    duration: '12:18',
    tag: '课程试看',
    description: '了解老师的讲课节奏、案例方式，以及九型如何落到关系与行动里。',
    poster: '/assets/teacher-poster.jpg',
    src: '',
  },
  {
    id: 'team',
    title: '企业团队 · 九型应用',
    duration: '10:06',
    tag: '企业课程',
    description: '面向团队管理、沟通协作和组织文化的九型应用说明。',
    poster: '/assets/teacher-mentor.jpg',
    src: '',
  },
]

export default function Watch() {
  const [activeId, setActiveId] = useState(VIDEOS[0].id)
  const active = VIDEOS.find((video) => video.id === activeId) || VIDEOS[0]

  return (
    <section className="watch wrap">
      <div className="watch__head">
        <Reveal>
          <p className="eyebrow">开始观看</p>
          <h1 className="display"><span className="gradient-text">九型芯之力<br />观看入口</span></h1>
          <p className="lead" style={{ marginTop: 18 }}>课程试看、老师介绍和活动回放统一放在这里。列表可以后续直接替换成真实视频地址。</p>
          <div className="btn-row">
            <a className="btn btn--red" href="/#signup">注册/报名咨询</a>
            <Link className="btn btn--ghost" to="/stages">先看九型的三阶段</Link>
          </div>
        </Reveal>
      </div>

      <div className="watch__grid">
        <Reveal className="watch-player panel">
          <div className="watch-player__screen">
            {active.src ? (
              <video key={active.id} controls preload="metadata" poster={active.poster}>
                <source src={active.src} type="video/mp4" />
              </video>
            ) : (
              <div className="watch-player__placeholder" style={{ backgroundImage: `url(${active.poster})` }}>
                <span className="watch-player__play">▶</span>
                <p>待接入视频文件</p>
              </div>
            )}
          </div>
          <div className="watch-player__meta">
            <span>{active.tag}</span>
            <span>{active.duration}</span>
          </div>
          <h2>{active.title}</h2>
          <p>{active.description}</p>
        </Reveal>

        <Reveal className="video-list card">
          <div className="video-list__head">
            <p className="kicker">视频列表</p>
            <span>{VIDEOS.length} 个视频</span>
          </div>
          {VIDEOS.map((video) => (
            <button
              key={video.id}
              className={video.id === active.id ? 'video-item active' : 'video-item'}
              onClick={() => setActiveId(video.id)}
            >
              <img src={video.poster} alt="" loading="lazy" />
              <span>
                <b>{video.title}</b>
                <small>{video.tag} · {video.duration}</small>
              </span>
            </button>
          ))}
        </Reveal>
      </div>
    </section>
  )
}
