import assert from 'node:assert/strict'
import { reportDisplayState } from './reportDisplayState.js'

assert.deepEqual(
  reportDisplayState({
    recordId: '',
    loading: false,
    error: '',
    unlocked: false,
    priceCents: null,
  }),
  { key: 'needs-save', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: 'r1',
    loading: true,
    error: '',
    unlocked: false,
    priceCents: null,
  }),
  { key: 'status-loading', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: 'r1',
    loading: false,
    error: '失败',
    unlocked: false,
    priceCents: null,
  }),
  { key: 'status-error', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: 'r1',
    loading: false,
    error: '',
    unlocked: false,
    priceCents: 990,
  }),
  { key: 'ready', priceCents: 990 },
)

for (const priceCents of [NaN, Infinity, -Infinity, -1, 0, '990']) {
  assert.deepEqual(
    reportDisplayState({
      recordId: 'r1',
      loading: false,
      error: '',
      unlocked: false,
      priceCents,
    }),
    { key: 'status-error', priceCents: null },
  )
}

assert.deepEqual(
  reportDisplayState({
    recordId: '',
    loading: true,
    error: '失败',
    unlocked: false,
    priceCents: 990,
  }),
  { key: 'needs-save', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: 'r1',
    loading: true,
    error: '失败',
    unlocked: false,
    priceCents: 990,
  }),
  { key: 'status-loading', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: 'r1',
    loading: false,
    error: '',
    unlocked: true,
    priceCents: null,
  }),
  { key: 'unlocked', priceCents: null },
)

assert.deepEqual(
  reportDisplayState({
    recordId: '',
    loading: true,
    error: '失败',
    unlocked: true,
    priceCents: null,
  }),
  { key: 'unlocked', priceCents: null },
)

console.log('report display state tests passed')
