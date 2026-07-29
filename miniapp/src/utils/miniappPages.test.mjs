import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const dir = await mkdtemp(join(tmpdir(), 'nx-miniapp-pages-'))
try {
  const modulePath = join(dir, 'miniappPages.mjs')
  await writeFile(modulePath, await readFile(new URL('./miniappPages.js', import.meta.url), 'utf8'))
  const { normalizeMiniappLearn } = await import(pathToFileURL(modulePath).href)

  const DEFAULT_LEARN = {
    hero: {
      eyebrow: '老师课堂',
      title: '跟着老师，把九型真正用进工作与生活',
      lead: '从视频与音频课件开始，理解自己、改善关系，也为团队协作建立更清晰的共同语言。',
      meta: ['视频课程', '音频精讲', '九型实践'],
    },
    classroom: {
      eyebrow: '课堂精选',
      title: '视频与音频课件',
      moreText: '查看全部',
      heroEyebrow: '随时回看 · 反复练习',
      heroTitle: '把老师以往开课内容，整理成可以持续学习的专业课件',
      heroLead: '支持视频和音频；先看独立课件，也可以进入系列课程循序学习。',
      ctaText: '进入老师课堂',
    },
    sections: {
      teacher: { eyebrow: '老师简介', title: '认识你的学习向导' },
      courses: { eyebrow: '课程方向', title: '循序建立九型视角' },
      types: { eyebrow: '九型内容', title: '九种性格，九条成长路径' },
      quotes: { eyebrow: '课堂一念', title: '把觉察带回当下' },
    },
    bottomCtaText: '先完成测试，建立你的学习地图',
  }

  assert.deepEqual(
    normalizeMiniappLearn(),
    DEFAULT_LEARN,
    'missing miniappLearn configuration should retain every current learn-page default',
  )

  const source = {
    home: {
      miniappLearn: {
        hero: { title: '  从一节课开始  ', meta: ['  视频课  ', '', 3, ' 音频课 ', '练习', '额外'] },
        classroom: { ctaText: '  去学习  ' },
        sections: { teacher: { title: '  认识导师  ' }, quotes: { eyebrow: '  今日一念  ' } },
        bottomCtaText: '  开始探索  ',
      },
    },
  }
  const before = structuredClone(source)
  assert.deepEqual(
    normalizeMiniappLearn(source),
    {
      hero: { ...DEFAULT_LEARN.hero, title: '从一节课开始', meta: ['视频课', '音频课', '练习'] },
      classroom: { ...DEFAULT_LEARN.classroom, ctaText: '去学习' },
      sections: {
        teacher: { ...DEFAULT_LEARN.sections.teacher, title: '认识导师' },
        courses: { ...DEFAULT_LEARN.sections.courses },
        types: { ...DEFAULT_LEARN.sections.types },
        quotes: { ...DEFAULT_LEARN.sections.quotes, eyebrow: '今日一念' },
      },
      bottomCtaText: '开始探索',
    },
    'valid nested fields should override independently while missing fields retain current defaults',
  )
  assert.deepEqual(source, before, 'normalization must not mutate the site-config response')

  const malformed = normalizeMiniappLearn({
    home: {
      miniappLearn: {
        hero: { eyebrow: ' ', title: 9, lead: null, meta: '视频课' },
        classroom: [],
        sections: { teacher: null, courses: { title: {} }, types: 'bad', quotes: { title: '' } },
        bottomCtaText: {},
      },
    },
  })
  assert.deepEqual(malformed, DEFAULT_LEARN, 'blank, malformed, and non-string values should recover per field')

  const one = normalizeMiniappLearn()
  const two = normalizeMiniappLearn()
  one.hero.meta[0] = 'changed'
  one.sections.teacher.title = 'changed'
  assert.deepEqual(two, DEFAULT_LEARN, 'results and nested defaults must be returned as independent copies')

  const throwingConfig = Object.defineProperty({}, 'home', {
    get() { throw new Error('malformed response getter') },
  })
  assert.deepEqual(normalizeMiniappLearn(throwingConfig), DEFAULT_LEARN, 'unexpected config access errors should use fresh defaults')

  console.log('miniapp learn normalization tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
