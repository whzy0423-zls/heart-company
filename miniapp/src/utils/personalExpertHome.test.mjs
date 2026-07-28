import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const dir = await mkdtemp(join(tmpdir(), 'nx-personal-expert-home-'))
try {
  const homeMenuPath = join(dir, 'homeMenu.mjs')
  const modulePath = join(dir, 'personalExpertHome.mjs')
  await writeFile(homeMenuPath, await readFile(new URL('./homeMenu.js', import.meta.url), 'utf8'))
  await writeFile(modulePath, (await readFile(new URL('./personalExpertHome.js', import.meta.url), 'utf8')).replace("'./homeMenu.js'", "'./homeMenu.mjs'"))

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
        eyebrow: '  创始导师  ', title: '  韩老师  ', lead: '  用九型看见真实动机  ', image: '  /teacher.png  ', fallbackImage: ' /fallback.png ',
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
    eyebrow: '创始导师', title: '韩老师', lead: '用九型看见真实动机', image: '/teacher.png', monogram: '九',
  }, 'teacherTeaser should take priority and trim image fields')
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
  }, 'enterprise items lead, then team-related courses preserve backend order')
  assert.deepEqual(view.game, {
    enabled: true, eyebrow: '开始探索', title: '你的专属测试', lead: '读懂你的核心动力', buttonText: '立即开始',
  })
  assert.deepEqual(view.secondaryEntries.map((entry) => entry.key), ['profile', 'relation', 'learn'], 'test should be isolated while remaining entries retain backend order and disabled state')
  assert.equal(view.secondaryEntries[0].enabled, false)
  assert.deepEqual(view.cases, [], 'no structured cases field means no fabricated social proof')
  assert.deepEqual(config, before, 'normalization must not mutate its input')

  const fallbackImage = normalizePersonalExpertHome({ home: { teacherTeaser: { title: '老师', fallbackImage: ' /fallback.png ' } } })
  assert.equal(fallbackImage.expertHero.image, '/fallback.png', 'teacher image should fall back without inventing a URL')
  assert.equal(fallbackImage.expertHero.monogram, '九')

  const rawTeacher = normalizePersonalExpertHome({ teacher: [{ name: '  林老师 ', title: ' 导师 ', image: ' /raw.png ', bio: ' 简介 ' }] })
  assert.deepEqual(rawTeacher.expertHero, { eyebrow: '导师', title: '林老师', lead: '简介', image: '/raw.png', monogram: '九' }, 'raw teacher is used when teaser is absent')

  const disabledGame = personalExpertGameSection({ home: { miniappHome: { entriesSection: { items: [{ key: 'test', enabled: false }] } } } })
  assert.equal(disabledGame.enabled, false, 'disabled test entry should disable the game')
  assert.equal(disabledGame.title, '人格测试', 'game falls back to legacy test entry copy')
  assert.equal(personalExpertProofStats({ home: { hero: { stats: [{ value: ' ', label: 'bad' }] } } }).length, 0, 'empty proof stats should stay empty rather than fabricate metrics')

  const defaults = personalExpertServices({ home: {} })
  assert.deepEqual(defaults.services, [
    { title: '企业团队共学', description: '用九型语言帮助团队看见协作中的动机与沟通方式。' },
  ], 'absent enterprise/course data should use stable non-numeric service copy')
  assert.equal(defaults.services.some((item) => /\d|客户|满意度/.test(`${item.title}${item.description}`)), false)

  const throwing = Object.defineProperty({}, 'home', { get() { throw new Error('bad getter') } })
  const first = normalizePersonalExpertHome(throwing)
  const second = normalizePersonalExpertHome(throwing)
  assert.deepEqual(first.proofStats, [])
  assert.deepEqual(first.cases, [])
  assert.notStrictEqual(first, second, 'errors should return fresh safe defaults')

  console.log('personal expert home normalization tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
