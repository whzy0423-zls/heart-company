// Teacher copy comes from the existing website Teacher page. Schedules and prices
// below are deliberately fictional and are consumed only by the UI preview.
export const STUDIO_TEACHER = {
  name: '韩常青', nickname: '老韩', title: '九型芯之力首席导师',
  avatar: '/static/teacher/portrait.jpg',
  bio: '北京九型成长平台、芯之力创始人。师从陈伟志博士，长期从事九型人格、教练技术、企业团队建设与家庭关系培训。把对性格的理解，带回真实的生活与关系。',
  tags: ['九型人格', '关系沟通', '个人成长'],
  timeline: [
    { year: '1999', title: '从自我探索开始', text: '开始学习教练技术，走进个人成长与教练训练体系。' },
    { year: '2004', title: '学习九型，持续传承', text: '跟随香港性格学导师陈伟志博士学习九型系统。' },
    { year: '至今', title: '让理解发生在生活里', text: '围绕个人、家庭与企业团队，开展课程、咨询和共学。' },
  ],
}

export const STUDIO_COURSES = [
  { id: 'intro', title: '九型人格与生命关系', subtitle: '看见自己，也读懂重要的人', description: '从九种核心动机出发，理解习惯性的反应，练习在关系里表达真实的自己。', cover: '/static/editorial/course-classroom.jpg', tag: '入门工作坊', format: '线下小班', duration: '2天', schedule: '10月17日—18日', location: '北京 · 朝阳', price: 1980, highlights: ['零基础友好', '情境互动练习', '课后觉察手册'], outline: ['认识九种核心动机与三大中心', '从生活情境中识别自己的反应模式', '看见亲密关系中的期待与需要', '带走一份可持续的日常练习计划'] },
  { id: 'relation', title: '性格情绪与亲子关系', subtitle: '从好好说话，到真正理解', description: '把注意力从行为对错转向情绪与需要，在真实的家庭情境中练习倾听和回应。', cover: '/static/editorial/course-relation.webp', tag: '关系共学', format: '线上共学', duration: '4次课', schedule: '10月24日起 · 每周六', location: '线上直播', price: 680, highlights: ['真实情境讨论', '每周家庭练习', '小组陪伴'], outline: ['理解情绪背后的需要', '辨认不同性格的沟通方式', '亲子冲突中的倾听与边界', '把新的沟通方式带回家庭'] },
  { id: 'growth', title: '九型人格与领导力', subtitle: '懂自己，才能带好团队', description: '用九型语言理解团队成员的动机，练习沟通反馈，在协作中建立共同语言。', cover: '/static/editorial/course-growth.webp', tag: '管理者工作坊', format: '线下工作坊', duration: '1天', schedule: '11月7日', location: '北京 · 朝阳', price: 1280, highlights: ['管理情境演练', '团队动机观察', '行动复盘'], outline: ['管理者的习惯与盲点', '理解团队成员的不同动机', '冲突中的反馈与对话', '制定团队协作行动计划'] },
]
