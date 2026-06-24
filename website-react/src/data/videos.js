const VIDEO_ITEMS = [
  ['laohan-01', '老韩短讲 01', '基础介绍', '00:28'],
  ['laohan-02', '老韩短讲 02', '九型观察', '00:50'],
  ['laohan-03', '老韩短讲 03', '成长片段', '00:51'],
  ['laohan-04', '老韩短讲 04', '关系理解', '00:50'],
  ['laohan-05', '老韩短讲 05', '课程片段', '00:57'],
  ['laohan-06', '老韩短讲 06', '性格能量', '00:39'],
  ['laohan-07', '老韩短讲 07', '九型应用', '00:45'],
  ['laohan-08', '老韩短讲 08', '学习现场', '00:39'],
  ['laohan-10', '老韩短讲 10', '成长提醒', '00:37'],
  ['laohan-11', '老韩短讲 11', '课程回放', '00:56'],
  ['laohan-13', '老韩短讲 13', '精选短讲', '00:31'],
  ['laohan-14', '老韩短讲 14', '沟通关系', '00:40'],
  ['laohan-15', '老韩短讲 15', '九型入门', '00:38'],
  ['laohan-16', '老韩短讲 16', '成长练习', '00:35'],
  ['laohan-17', '老韩短讲 17', '课程片段', '00:54'],
  ['laohan-18', '老韩短讲 18', '关系课堂', '00:50'],
  ['laohan-19', '老韩短讲 19', '性格解读', '00:49'],
  ['laohan-20', '老韩短讲 20', '团队应用', '00:47'],
  ['laohan-22', '老韩短讲 22', '九型现场', '00:49'],
]

export const VIDEOS = VIDEO_ITEMS.map(([id, title, tag, duration]) => ({
  id,
  title,
  tag,
  duration,
  description: '老韩老师围绕九型人格、性格能量与关系成长的短视频片段。',
  poster: `/assets/videos/posters/${id}.jpg`,
  src: `/assets/videos/${id}.mp4`,
}))

const FEATURED_IDS = new Set(['laohan-01', 'laohan-13', 'laohan-20'])

export const FEATURED_VIDEOS = VIDEOS.filter((video) => FEATURED_IDS.has(video.id))
