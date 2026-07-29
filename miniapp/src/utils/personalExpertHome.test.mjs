import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const dir = await mkdtemp(join(tmpdir(), 'nx-personal-expert-home-'))
try {
  const homeMenuPath = join(dir, 'homeMenu.mjs')
  const contentAssetPath = join(dir, 'contentAsset.mjs')
  const modulePath = join(dir, 'personalExpertHome.mjs')
  await writeFile(homeMenuPath, await readFile(new URL('./homeMenu.js', import.meta.url), 'utf8'))
  await writeFile(
    contentAssetPath,
    (await readFile(new URL('./contentAsset.js', import.meta.url), 'utf8'))
      .replace(
        /import \{ API_BASE(?:, DEFAULT_API_BASE)? \} from '\.\.\/config'/,
        "const API_BASE = 'https://api.example.test/api'; const DEFAULT_API_BASE = API_BASE",
      ),
  )
  await writeFile(
    modulePath,
    (await readFile(new URL('./personalExpertHome.js', import.meta.url), 'utf8'))
      .replace("'./homeMenu.js'", "'./homeMenu.mjs'")
      .replace("'./contentAsset.js'", "'./contentAsset.mjs'"),
  )

  const {
    normalizePersonalExpertHome,
    personalExpertGameSection,
    personalExpertProofStats,
    personalExpertServices,
  } = await import(pathToFileURL(modulePath).href)

  const config = {
    home: {
      miniappHome: {
        brand: { name: '  九型·韩老师  ' },
        entriesSection: { items: [
          { key: 'test', enabled: true, title: '  你的人设出厂设置  ', description: '  18题找到你的内在驱动  ' },
          { key: 'profile', enabled: false },
          { key: 'relation', enabled: true },
          { key: 'learn', enabled: true },
        ] },
      },
      teacherTeaser: {
        eyebrow: '  创始导师  ', title: '  韩老师  ', lead: '  用九型看见真实动机  ',
        portraitImage: '  /assets/teacher.jpg  ', avatar: ' /static/avatar.png ',
        detailImage: '  /assets/teacher-poster.jpg  ', poster: ' /static/poster.png ', image: ' /static/legacy-poster.png ',
      },
      hero: { stats: [
        { value: ' 96 ', suffix: ' % ', label: ' 学员好评 ' },
        { value: '  ', suffix: '次', label: '无效' },
        { value: '80+', suffix: '', label: ' 课程交付 ' },
        { value: 'too-many', suffix: '', label: '忽略' },
      ] },
      enterprise: {
        eyebrow: '  企业共创  ', title: '  把理解带进团队  ', lead: '  面向组织的九型工作坊  ', buttonText: '  预约沟通  ',
        modules: [' 团队协作 ', ' 管理者觉察 '],
        items: [{ title: ' 企业沟通工作坊 ', description: ' 从冲突中建立理解 ' }],
      },
      courses: { items: [
        { title: '九型入门课', description: '个人学习' },
        { title: ' 领导力与团队协作 ', description: ' 团队课程 ' },
      ] },
      game: { eyebrow: '  开始探索  ', title: '  你的专属测试  ', lead: '  读懂你的核心动力  ', buttonText: '  立即开始  ' },
    },
    teachers: [{ name: '不应覆盖 teaser' }],
  }
  const before = structuredClone(config)
  const view = normalizePersonalExpertHome(config)

  assert.deepEqual(view.brand, { enabled: true, name: '九型·韩老师', tagline: '看见动机，找到成长方向' })
  assert.deepEqual(view.expertHero, {
    eyebrow: '创始导师', title: '韩老师', lead: '用九型看见真实动机',
    portraitImage: 'https://api.example.test/assets/teacher.jpg',
    detailImage: 'https://api.example.test/assets/teacher-poster.jpg',
    image: 'https://api.example.test/assets/teacher-poster.jpg', monogram: '九',
  }, 'teacherTeaser should prioritize and resolve separate portrait and detail image fields')
  assert.deepEqual(view.proofStats, [
    { value: '96', suffix: '%', label: '学员好评' },
    { value: '80+', suffix: '', label: '课程交付' },
    { value: 'too-many', suffix: '', label: '忽略' },
  ], 'only complete configured stats should be retained, capped at three')
  assert.deepEqual(view.enterprise, {
    eyebrow: '企业共创', title: '把理解带进团队', lead: '面向组织的九型工作坊', buttonText: '预约沟通',
    modules: ['团队协作', '管理者觉察'],
    services: [
      { title: '企业沟通工作坊', description: '从冲突中建立理解' },
      { title: '领导力与团队协作', description: '团队课程' },
    ],
    serviceModes: [
      { title: '企业沟通工作坊', description: '从冲突中建立理解' },
    ],
    processSteps: [
      { title: '需求沟通', description: '先了解团队背景、参与对象和希望解决的问题。' },
      { title: '方案共创', description: '结合九型主题、课件内容和企业节奏设计服务方式。' },
      { title: '落地交付', description: '完成课程或工作坊后，沉淀可复盘的团队语言。' },
    ],
  }, 'enterprise items lead, then team-related courses preserve backend order')
  assert.deepEqual(view.game, {
    enabled: true, eyebrow: '开始探索', title: '你的专属测试', lead: '读懂你的核心动力', buttonText: '立即开始',
  })
  assert.deepEqual(view.secondaryEntries.map((entry) => entry.key), ['profile', 'relation', 'learn'], 'test should be isolated while remaining entries retain backend order and disabled state')
  assert.equal(view.secondaryEntries[0].enabled, false)
  assert.deepEqual(view.cases, [], 'no structured cases field means no fabricated social proof')
  assert.deepEqual(config, before, 'normalization must not mutate its input')

  const defaultImages = normalizePersonalExpertHome({ home: { teacherTeaser: { title: '老师' } } })
  assert.equal(defaultImages.expertHero.portraitImage, 'https://api.example.test/assets/teacher.jpg')
  assert.equal(defaultImages.expertHero.detailImage, 'https://api.example.test/assets/teacher-poster.jpg')
  assert.equal(defaultImages.expertHero.image, defaultImages.expertHero.detailImage, 'legacy image should alias the detail image')
  assert.equal(defaultImages.expertHero.monogram, '九')

  const invalidPrimaryImages = normalizePersonalExpertHome({
    home: { teacherTeaser: {
      title: '老师', portraitImage: '/legacy-portrait.png', avatar: '/static/avatar.png', photo: '/static/photo.png',
      detailImage: '/legacy-detail.png', poster: '/static/poster.png', image: '/static/legacy-poster.png',
    } },
  })
  assert.equal(
    invalidPrimaryImages.expertHero.portraitImage,
    '/static/avatar.png',
    'an unsupported portraitImage must not block a valid avatar candidate',
  )
  assert.equal(
    invalidPrimaryImages.expertHero.detailImage,
    '/static/poster.png',
    'an unsupported detailImage must not block a valid poster candidate',
  )
  assert.equal(invalidPrimaryImages.expertHero.image, invalidPrimaryImages.expertHero.detailImage)

  const legacyDetailImage = normalizePersonalExpertHome({
    home: { teacherTeaser: { title: '老师', detailImage: '/bad-detail.png', poster: '/bad-poster.png', image: '/static/legacy-poster.png' } },
  })
  assert.equal(legacyDetailImage.expertHero.detailImage, '/static/legacy-poster.png', 'legacy image should remain a detail-image candidate')

  const coverDetailImage = normalizePersonalExpertHome({
    home: { teacherTeaser: {
      title: '老师', detailImage: '/bad-detail.png', poster: '/bad-poster.png', image: '/bad-image.png', cover: '/static/cover.png', fallbackImage: '/static/fallback.png',
    } },
  })
  assert.equal(coverDetailImage.expertHero.detailImage, '/static/cover.png', 'cover should be tried after legacy image')

  const fallbackDetailImage = normalizePersonalExpertHome({
    home: { teacherTeaser: {
      title: '老师', detailImage: '/bad-detail.png', poster: '/bad-poster.png', image: '/bad-image.png', cover: '/bad-cover.png', fallbackImage: '/static/fallback.png',
    } },
  })
  assert.equal(fallbackDetailImage.expertHero.detailImage, '/static/fallback.png', 'fallbackImage should remain a compatible detail candidate')
  assert.equal(fallbackDetailImage.expertHero.image, fallbackDetailImage.expertHero.detailImage)

  const rawPortrait = normalizePersonalExpertHome({ teacher: [{ name: '老师', portraitImage: '/static/portrait.png', avatar: '/static/avatar.png', photo: '/static/photo.png', cover: '/static/cover.png' }] })
  assert.equal(rawPortrait.expertHero.portraitImage, '/static/portrait.png', 'raw teacher portraitImage should have highest priority')
  const rawAvatar = normalizePersonalExpertHome({ teacher: [{ name: '老师', portraitImage: '/bad-portrait.png', avatar: '/static/avatar.png', photo: '/static/photo.png', cover: '/static/cover.png' }] })
  assert.equal(rawAvatar.expertHero.portraitImage, '/static/avatar.png', 'raw teacher avatar should follow portraitImage')
  const rawPhoto = normalizePersonalExpertHome({ teacher: [{ name: '老师', avatar: '/bad-avatar.png', photo: '/static/photo.png', cover: '/static/cover.png' }] })
  assert.equal(rawPhoto.expertHero.portraitImage, '/static/photo.png', 'raw teacher photo should follow avatar')
  const rawCover = normalizePersonalExpertHome({ teacher: [{ name: '老师', avatar: '/bad-avatar.png', photo: '/bad-photo.png', cover: '/static/cover.png' }] })
  assert.equal(rawCover.expertHero.portraitImage, '/static/cover.png', 'raw teacher cover should follow photo')
  const rawDefault = normalizePersonalExpertHome({ teacher: [{ name: '老师', avatar: '/bad-avatar.png', photo: '/bad-photo.png', cover: '/bad-cover.png' }] })
  assert.equal(rawDefault.expertHero.portraitImage, 'https://api.example.test/assets/teacher.jpg', 'raw teacher should use the default portrait when candidates are invalid')

  const rawTeacher = normalizePersonalExpertHome({ teacher: [{ name: '  林老师 ', title: ' 导师 ', image: ' /static/raw.png ', bio: ' 简介 ' }] })
  assert.deepEqual(rawTeacher.expertHero, {
    eyebrow: '导师', title: '林老师', lead: '简介',
    portraitImage: 'https://api.example.test/assets/teacher.jpg',
    detailImage: '/static/raw.png', image: '/static/raw.png', monogram: '九',
  }, 'raw teacher is used when teaser is absent and legacy image remains the detail source')

  const disabledGame = personalExpertGameSection({ home: { miniappHome: { entriesSection: { items: [{ key: 'test', enabled: false }] } } } })
  assert.equal(disabledGame.enabled, false, 'disabled test entry should disable the game')
  assert.equal(disabledGame.title, '人格测试', 'game falls back to legacy test entry copy')
  assert.equal(personalExpertProofStats({ home: { hero: { stats: [{ value: ' ', label: 'bad' }] } } }).length, 0, 'empty proof stats should stay empty rather than fabricate metrics')

  const defaults = personalExpertServices({ home: {} })
  assert.deepEqual(defaults.services, [
    { title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' },
  ], 'absent enterprise/course data should use stable non-numeric service copy')
  assert.equal(defaults.services.some((item) => /\d|客户|满意度/.test(`${item.title}${item.description}`)), false)

  const enterpriseBookingConfig = {
    home: {
      enterprise: {
        items: [
          { title: ' 企业内训 ', description: ' 围绕组织议题共学 ' },
          {},
          { title: ' 团队工作坊 ', description: ' 建立协作共识 ' },
          { title: ' 管理者培训 ', description: ' 识别成员动机 ' },
          { title: ' 领导力共学 ', description: ' 支持管理升级 ' },
          { title: ' 应被截断 ', description: ' 第五项 ' },
        ],
        processSteps: [
          { title: ' 需求澄清 ', description: ' 了解团队背景 ' },
          {},
          { title: ' 共创方案 ', description: ' 匹配主题与节奏 ' },
          { title: ' 现场交付 ', description: ' 沉淀团队语言 ' },
          { title: ' 复盘跟进 ', description: ' 形成下一步 ' },
          { title: ' 应被截断 ', description: ' 第五步 ' },
        ],
      },
    },
  }
  const enterpriseBookingBefore = structuredClone(enterpriseBookingConfig)
  const enterpriseBooking = personalExpertServices(enterpriseBookingConfig)
  assert.deepEqual(enterpriseBooking.serviceModes, [
    { title: '企业内训', description: '围绕组织议题共学' },
    { title: '团队工作坊', description: '建立协作共识' },
    { title: '管理者培训', description: '识别成员动机' },
    { title: '领导力共学', description: '支持管理升级' },
  ], 'enterprise items should provide ordered, trimmed service modes capped at four')
  assert.deepEqual(enterpriseBooking.processSteps, [
    { title: '需求澄清', description: '了解团队背景' },
    { title: '共创方案', description: '匹配主题与节奏' },
    { title: '现场交付', description: '沉淀团队语言' },
    { title: '复盘跟进', description: '形成下一步' },
  ], 'configured process steps should be ordered, trimmed, filtered, and capped at four')
  assert.deepEqual(enterpriseBookingConfig, enterpriseBookingBefore, 'enterprise booking normalization must not mutate its input')

  const enterpriseBookingDefaults = personalExpertServices({ home: { enterprise: { items: [{}, { title: ' ', description: ' ' }], processSteps: [{}] } } })
  assert.deepEqual(enterpriseBookingDefaults.serviceModes, [
    { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
    { title: '团队工作坊', description: '用互动练习帮助团队建立沟通和协作共识。' },
    { title: '管理者培训', description: '支持管理者识别不同类型成员的动机与压力反应。' },
  ], 'missing or invalid enterprise items should use the stable booking service mode defaults')
  assert.deepEqual(enterpriseBookingDefaults.processSteps, [
    { title: '需求沟通', description: '先了解团队背景、参与对象和希望解决的问题。' },
    { title: '方案共创', description: '结合九型主题、课件内容和企业节奏设计服务方式。' },
    { title: '落地交付', description: '完成课程或工作坊后，沉淀可复盘的团队语言。' },
  ], 'missing or invalid process steps should use the stable booking process defaults')

  const throwing = Object.defineProperty({}, 'home', { get() { throw new Error('bad getter') } })
  const first = normalizePersonalExpertHome(throwing)
  const second = normalizePersonalExpertHome(throwing)
  assert.deepEqual(first.proofStats, [])
  assert.deepEqual(first.cases, [])
  assert.deepEqual(first.enterprise.serviceModes, [
    { title: '企业内训', description: '围绕企业当下议题设计半天或全天共学。' },
    { title: '团队工作坊', description: '用互动练习帮助团队建立沟通和协作共识。' },
    { title: '管理者培训', description: '支持管理者识别不同类型成员的动机与压力反应。' },
  ], 'error getters should return booking service mode defaults')
  assert.notStrictEqual(first, second, 'errors should return fresh safe defaults')
  assert.notStrictEqual(first.enterprise.serviceModes, second.enterprise.serviceModes, 'error getters should create fresh mode defaults')
  assert.notStrictEqual(first.enterprise.processSteps, second.enterprise.processSteps, 'error getters should create fresh process defaults')

  const boundedServices = personalExpertServices({
    home: {
      enterprise: {
        modules: [' 模块一 ', '模块二', '模块三', '模块四', '模块五'],
        items: [
          { title: '服务一', description: '说明一' }, { title: '服务二', description: '说明二' },
          { title: '服务三', description: '说明三' }, { title: '服务四', description: '说明四' },
        ],
      },
      courses: { items: [
        { title: '个人入门课', tags: ['enterprise'], summary: '普通标题但由企业标签标记' },
        { title: '团队协作', description: '团队课程' }, { title: '领导力', category: 'leadership' },
        { title: '企业工作坊', badge: 'workshop' }, { title: '组织沟通', type: 'organization' },
      ] },
    },
  })
  assert.deepEqual(boundedServices.modules, ['模块一', '模块二', '模块三', '模块四'], 'enterprise modules should cap at four in backend order')
  assert.deepEqual(
    boundedServices.services,
    [
      { title: '服务一', description: '说明一' }, { title: '服务二', description: '说明二' }, { title: '服务三', description: '说明三' },
    ],
    'configured enterprise service items should cap at three before courses',
  )

  const metadataMatchedCourses = personalExpertServices({
    home: { courses: { items: [
      { title: '个人入门课', tags: ['enterprise'], summary: '普通标题但由企业标签标记' },
      { title: '团队协作', description: '团队课程' }, { title: '领导力', category: 'leadership' },
      { title: '企业工作坊', badge: 'workshop' },
    ] } },
  })
  assert.deepEqual(
    metadataMatchedCourses.services,
    [
      { title: '个人入门课', description: '普通标题但由企业标签标记' },
      { title: '团队协作', description: '团队课程' },
      { title: '领导力', description: '围绕团队协作、沟通与领导力的九型共学。' },
    ],
    'metadata-tagged courses should be selected in order and capped at three',
  )
  assert.deepEqual(
    personalExpertServices({ home: { courses: { items: [{ title: '九型入门', description: '个人学习' }] } } }).services,
    [{ title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' }],
    'unmatched personal courses should not be presented as enterprise services',
  )

  console.log('personal expert home normalization tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
