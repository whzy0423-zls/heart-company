import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-home-carousel-'))
try {
const modulePath = join(dir, 'homeCarousel.mjs')
let source = await readFile(new URL('./homeCarousel.js', import.meta.url), 'utf8')
source = source.replace("import { API_BASE } from '../config'", "const API_BASE = 'https://example.test/api'")
await writeFile(modulePath, source)

const { filterFailedCarouselItems, normalizeHomeCarousel } = await import(`file://${modulePath}`)

assert.deepEqual(
  normalizeHomeCarousel(),
  { autoplay: true, interval: 4000, items: [] },
  'missing configuration should use safe carousel defaults',
)
assert.deepEqual(
  normalizeHomeCarousel(null),
  { autoplay: true, interval: 4000, items: [] },
  'invalid configuration should use safe carousel defaults',
)

const filtered = normalizeHomeCarousel({
  home: {
    miniappCarousel: {
      autoplay: false,
      interval: 1500,
      items: [
        { image: '  /static/first.png  ' },
        { image: ' ', title: 'empty image is ignored' },
        { enabled: false, image: '/static/disabled.png' },
        { image: 'https://cdn.example.com/second.png', title: 'kept' },
        { image: '/api/uploads/third.png' },
      ],
    },
  },
}, { apiBase: 'https://api.example.com/api' })
assert.deepEqual(
  filtered,
  {
    autoplay: false,
    interval: 2000,
    items: [
      { image: '/static/first.png' },
      { image: 'https://cdn.example.com/second.png' },
      { image: 'https://api.example.com/api/uploads/third.png' },
    ],
  },
  'enabled images should keep order, trim paths, and resolve API uploads',
)

assert.deepEqual(
  normalizeHomeCarousel({
    home: {
      miniappCarousel: {
        items: [
          { image: '/api/uploads/banner.png' },
          { image: 'https://api.example.com/api/uploads/banner.png' },
          { image: '/static/other-banner.png' },
          { image: '/static/other-banner.png' },
        ],
      },
    },
  }, { apiBase: 'https://api.example.com/api' }).items,
  [
    { image: 'https://api.example.com/api/uploads/banner.png' },
    { image: '/static/other-banner.png' },
  ],
  'duplicate final image URLs should be removed while preserving their first appearance',
)

const carouselBeforeRefresh = {
  autoplay: true,
  interval: 4000,
  items: [
    { image: '/static/failed.png' },
    { image: '/static/available.png' },
  ],
}
assert.deepEqual(
  filterFailedCarouselItems(carouselBeforeRefresh, new Set(['/static/failed.png'])),
  {
    autoplay: true,
    interval: 4000,
    items: [{ image: '/static/available.png' }],
  },
  'failed image URLs should remain excluded when a later cache or refresh result is applied',
)
assert.deepEqual(
  carouselBeforeRefresh.items,
  [
    { image: '/static/failed.png' },
    { image: '/static/available.png' },
  ],
  'filtering failed carousel images should not mutate the refresh input',
)

assert.equal(
  normalizeHomeCarousel({ home: { miniappCarousel: { interval: 15000 } } }).interval,
  10000,
  'interval should clamp to the maximum supported value',
)
assert.equal(
  normalizeHomeCarousel({ home: { miniappCarousel: { interval: Number.POSITIVE_INFINITY } } }).interval,
  4000,
  'non-finite interval should use the default',
)
assert.equal(
  normalizeHomeCarousel({ home: { miniappCarousel: { autoplay: 0 } } }).autoplay,
  true,
  'only an explicit false should disable autoplay',
)
assert.deepEqual(
  normalizeHomeCarousel({
    home: { miniappCarousel: { items: [{ image: '/api/banner.png' }, { image: '/pages/local.png' }, { image: 'http://cdn.example.com/banner.png' }] } },
  }, { apiBase: 'https://host.example/api/' }).items,
  [
    { image: 'https://host.example/api/banner.png' },
    { image: '/pages/local.png' },
    { image: 'http://cdn.example.com/banner.png' },
  ],
  'API paths should use the API host while local and absolute URLs remain loadable',
)

assert.deepEqual(
  normalizeHomeCarousel({
    home: { miniappCarousel: { items: [{ image: '/api/kept-with-null-options.png' }] } },
  }, null).items,
  [{ image: 'https://example.test/api/kept-with-null-options.png' }],
  'null options should retain valid carousel items and use the default API base',
)
assert.deepEqual(
  normalizeHomeCarousel({
    home: { miniappCarousel: { items: [{ image: 123 }, { image: {} }, { image: '/static/valid.png' }] } },
  }).items,
  [{ image: '/static/valid.png' }],
  'only non-empty string image paths should be retained',
)

console.log('home carousel normalization tests passed')
} finally {
await rm(dir, { force: true, recursive: true })
}
