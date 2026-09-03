import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { fileURLToPath } from 'node:url'
import { execFileSync } from 'node:child_process'

const repo = fileURLToPath(new URL('../..', import.meta.url))
const baseline = join(repo, 'tmp/website-react-before.sha256')
const oldRoot = join(repo, 'website-react')
const ignored = new Set(['node_modules', 'dist', '.vite', '.ace-tool', '.superpowers'])
const ignoredPrefixes = ['public/assets/uploads/']

if (!existsSync(baseline)) throw new Error(`missing baseline: ${baseline}`)

function shouldInclude(path) {
  const normalized = path.split('\\').join('/')
  return !ignored.has(normalized.split('/')[0]) && !ignoredPrefixes.some((prefix) => normalized.startsWith(prefix))
}

const current = execFileSync('find', [oldRoot, '-type', 'f', '-print0'], { encoding: 'buffer' })
const paths = current.toString().split('\0').filter(Boolean).map((path) => relative(oldRoot, path)).filter(shouldInclude).sort()
const currentLines = paths.map((path) => {
  const digest = createHash('sha256').update(readFileSync(join(oldRoot, path))).digest('hex')
  return `${digest}  ${join('website-react', path)}`
})
const expected = readFileSync(baseline, 'utf8').trim().split('\n').filter(Boolean).sort()
const actual = currentLines.map((line) => line.replace(repo + '/', '')).sort()
if (expected.length !== actual.length || expected.some((line, index) => line !== actual[index])) {
  const expectedSet = new Set(expected)
  const actualSet = new Set(actual)
  const added = actual.filter((line) => !expectedSet.has(line))
  const removed = expected.filter((line) => !actualSet.has(line))
  throw new Error(`old website changed. added=${added.join(',')} removed=${removed.join(',')}`)
}
console.log(`old website unchanged: ${actual.length} files verified`)
