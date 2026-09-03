import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const mainSource = readFileSync(resolve(__dirname, 'main.jsx'), 'utf8')
const appSource = readFileSync(resolve(__dirname, 'App.jsx'), 'utf8')

assert.match(
  mainSource,
  /const\s+root\s*=\s*createRoot\(document\.getElementById\('root'\)\)/,
  '官网入口需要先创建 root，避免等待远程配置后才开始渲染',
)

const firstRender = mainSource.indexOf('renderApp()')
const configHydrate = mainSource.indexOf('hydrateSiteConfigFromBackend()')
assert.ok(firstRender >= 0, '官网入口需要立即调用 renderApp()')
assert.ok(configHydrate >= 0, '官网入口需要在后台拉取站点配置')
assert.ok(
  firstRender < configHydrate,
  '首屏渲染必须早于后台配置拉取，避免 /api/public/site-config 慢时白屏',
)

const awaitedConfigFetch = mainSource.indexOf('await fetch(`${API_BASE}/public/site-config`')
assert.ok(awaitedConfigFetch >= 0, '后台配置拉取仍需要使用 fetch 更新站点配置')
assert.ok(
  firstRender < awaitedConfigFetch,
  '即使后台配置函数内部 await fetch，也必须发生在首屏 renderApp() 调用之后',
)

assert.doesNotMatch(
  appSource,
  /fallback=\{null\}/,
  '懒加载路由不能使用空 fallback，慢网切页需要可见加载态',
)

assert.match(
  appSource,
  /className="route-loading"/,
  '懒加载路由需要提供 route-loading 可见加载态',
)

assert.match(
  mainSource,
  /v7_startTransition:\s*true/,
  'BrowserRouter 需要提前启用 v7_startTransition，避免开发环境控制台警告',
)

assert.match(
  mainSource,
  /v7_relativeSplatPath:\s*true/,
  'BrowserRouter 需要提前启用 v7_relativeSplatPath，避免开发环境控制台警告',
)

assert.doesNotMatch(
  appSource,
  /element=\{\{lazyRoute/,
  '懒加载路由 element 只能传 ReactNode，不能写成双花括号对象',
)

console.log('website bootstrap resilience tests passed')
