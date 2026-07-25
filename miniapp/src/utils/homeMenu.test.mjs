import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const dir = await mkdtemp(join(tmpdir(), 'nx-home-menu-'))
try {
  const modulePath = join(dir, 'homeMenu.mjs')
  await writeFile(modulePath, await readFile(new URL('./homeMenu.js', import.meta.url), 'utf8'))

  const {
    MINIAPP_HOME_ENTRY_BEHAVIORS,
    MINIAPP_HOME_ICON_KEYS,
    MINIAPP_HOME_THEME_KEYS,
    normalizeMiniappHome,
  } = await import(pathToFileURL(modulePath).href)

  const DEFAULT_HOME = {
    brand: {
      enabled: true,
      name: '九型芯之力',
      tagline: '看见动机，找到成长方向',
    },
    hero: {
      enabled: true,
      kicker: '老师导学 · 课程配套 · 18 题自测',
      title: '读懂自己内在的能量地图',
      description: '从核心动机出发，在老师课程中理解自己，也更从容地走进关系与成长。',
      buttonText: '开始人格测试',
    },
    entriesSection: {
      enabled: true,
      title: '探索你的九型能量',
      description: '从测试、关系、课程到成长档案，选择此刻最需要的一步。',
      items: [
        { key: 'test', enabled: true, title: '人格测试', description: '找到你的核心动机', icon: 'compass', theme: 'blue' },
        { key: 'relation', enabled: true, title: '关系合盘', description: '看见彼此的互动模式', icon: 'relation', theme: 'purple' },
        { key: 'learn', enabled: true, title: '老师课程', description: '跟着课件系统学习', icon: 'book', theme: 'orange' },
        { key: 'profile', enabled: true, title: '成长档案', description: '记录你的探索轨迹', icon: 'growth', theme: 'pink' },
      ],
    },
    growth: {
      enabled: true,
      eyebrow: '老师陪伴 · 持续成长',
      title: '把测试发现带进课程练习',
      description: '跟随老师的课程与课件，让理解沉淀为真实的成长行动。',
    },
  }

  assert.deepEqual(
    normalizeMiniappHome(),
    DEFAULT_HOME,
    'missing configuration should preserve every current purple-home default',
  )

  assert.deepEqual(
    normalizeMiniappHome({ home: { miniappHome: { brand: null, hero: [], entriesSection: 'bad', growth: 7 } } }),
    DEFAULT_HOME,
    'missing or malformed sections should recover independently to complete defaults',
  )

  const emptyAndInvalid = normalizeMiniappHome({
    home: {
      miniappHome: {
        brand: { enabled: 'false', name: '', tagline: '   ' },
        hero: { enabled: 0, kicker: null, title: '', description: {}, buttonText: [] },
        entriesSection: { enabled: 'yes', title: '', description: '\n', items: [] },
        growth: { enabled: null, eyebrow: '', title: '  ', description: false },
      },
    },
  })
  assert.deepEqual(
    emptyAndInvalid,
    DEFAULT_HOME,
    'empty copy and non-boolean enabled values should use defaults rather than coercion',
  )

  const normalizedEntries = normalizeMiniappHome({
    home: {
      miniappHome: {
        entriesSection: {
          title: '  自定义入口  ',
          description: '  自定义说明  ',
          items: [
            { key: 'profile', enabled: false, title: '  我的记录  ', description: '  看见变化  ', icon: 'heart', theme: 'cyan' },
            { key: 'unknown', enabled: true, title: 'unknown entry' },
            { key: 'profile', enabled: true, title: 'duplicate entry' },
            { key: 'test', enabled: 'false', title: '', description: '', icon: 'free-form-icon', theme: '#ff00ff' },
            null,
          ],
        },
      },
    },
  })
  assert.equal(normalizedEntries.entriesSection.title, '自定义入口', 'valid copy should be trimmed')
  assert.equal(normalizedEntries.entriesSection.description, '自定义说明', 'valid section copy should be trimmed')
  assert.deepEqual(
    normalizedEntries.entriesSection.items,
    [
      { key: 'profile', enabled: false, title: '我的记录', description: '看见变化', icon: 'heart', theme: 'cyan' },
      { key: 'test', enabled: true, title: '人格测试', description: '找到你的核心动机', icon: 'compass', theme: 'blue' },
      { key: 'relation', enabled: true, title: '关系合盘', description: '看见彼此的互动模式', icon: 'relation', theme: 'purple' },
      { key: 'learn', enabled: true, title: '老师课程', description: '跟着课件系统学习', icon: 'book', theme: 'orange' },
    ],
    'configured order should survive while duplicate/unknown keys are dropped and missing fixed entries append',
  )

  const allDisabled = normalizeMiniappHome({
    home: {
      miniappHome: {
        entriesSection: {
          enabled: true,
          items: DEFAULT_HOME.entriesSection.items.map((item) => ({ ...item, enabled: false })),
        },
      },
    },
  })
  assert.equal(allDisabled.entriesSection.enabled, false, 'an all-disabled entry list should hide its section')
  assert.equal(allDisabled.entriesSection.items.every((item) => item.enabled === false), true, 'disabled entry state should remain available')

  const explicitlyDisabled = normalizeMiniappHome({
    home: {
      miniappHome: {
        brand: { enabled: false },
        hero: { enabled: false },
        entriesSection: { enabled: false },
        growth: { enabled: false },
      },
    },
  })
  assert.equal(explicitlyDisabled.brand.enabled, false, 'explicit false should disable the brand section')
  assert.equal(explicitlyDisabled.hero.enabled, false, 'explicit false should disable the hero section')
  assert.equal(explicitlyDisabled.entriesSection.enabled, false, 'explicit false should disable the entries section')
  assert.equal(explicitlyDisabled.growth.enabled, false, 'explicit false should disable the growth section')

  assert.deepEqual(
    MINIAPP_HOME_ICON_KEYS,
    ['compass', 'relation', 'book', 'growth', 'spark', 'heart'],
    'icon presets should be a closed allowlist',
  )
  assert.deepEqual(
    MINIAPP_HOME_THEME_KEYS,
    ['blue', 'purple', 'orange', 'pink', 'cyan'],
    'theme presets should be a closed allowlist',
  )
  assert.deepEqual(
    MINIAPP_HOME_ENTRY_BEHAVIORS,
    {
      test: { method: 'navigateTo', url: '/pages/test/test', ariaLabel: '开始九型人格测试' },
      relation: { method: 'navigateTo', url: '/pages/relation/relation', ariaLabel: '打开九型关系合盘' },
      learn: { method: 'switchTab', url: '/pages/learn/learn', ariaLabel: '打开老师课程与课件' },
      profile: { method: 'switchTab', url: '/pages/profile/profile', ariaLabel: '打开我的成长档案' },
    },
    'fixed entry metadata should own navigation behavior instead of accepting configurable URLs',
  )

  const input = {
    home: {
      untouched: { keep: true },
      miniappHome: {
        brand: { name: '自定义品牌' },
        entriesSection: { items: [{ key: 'learn', title: '自定义课程' }] },
      },
    },
  }
  const before = structuredClone(input)
  const first = normalizeMiniappHome(input)
  first.brand.name = 'mutated result'
  first.entriesSection.items[0].title = 'mutated result item'
  assert.deepEqual(input, before, 'normalization and returned-object edits should never mutate the API response')

  const afterResultMutation = normalizeMiniappHome()
  assert.deepEqual(afterResultMutation, DEFAULT_HOME, 'mutating one normalized result should not pollute later defaults')
  assert.notStrictEqual(first.brand, afterResultMutation.brand, 'each normalization should return an independent brand object')
  assert.notStrictEqual(first.entriesSection.items, afterResultMutation.entriesSection.items, 'each normalization should return an independent item array')
  assert.notStrictEqual(first.entriesSection.items[0], afterResultMutation.entriesSection.items[0], 'entry objects should not share nested references')

  const throwingConfig = Object.defineProperty({}, 'home', {
    get() {
      throw new Error('malformed response getter')
    },
  })
  assert.deepEqual(
    normalizeMiniappHome(throwingConfig),
    DEFAULT_HOME,
    'unexpected response access errors should return a fresh safe default',
  )

  console.log('home menu normalization tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
