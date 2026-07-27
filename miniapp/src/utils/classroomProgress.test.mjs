import assert from 'node:assert/strict'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const dir = await mkdtemp(join(tmpdir(), 'nx-classroom-progress-'))
try {
  const source = await readFile(new URL('./classroomProgress.js', import.meta.url), 'utf8')
  const modulePath = join(dir, 'classroomProgress.mjs')
  await writeFile(modulePath, source)
  const {
    CLASSROOM_PROGRESS_THROTTLE_MS,
    classroomCompletion,
    createClassroomProgressTracker,
    readAnonymousClassroomProgress,
  } = await import(`file://${modulePath}`)

  assert.equal(CLASSROOM_PROGRESS_THROTTLE_MS, 12_000)
  assert.deepEqual(classroomCompletion(89, 100), { ratio: 0.89, completed: false })
  assert.deepEqual(classroomCompletion(90, 100), { ratio: 0.9, completed: true })
  assert.deepEqual(classroomCompletion(999, 100), { ratio: 1, completed: true })
  assert.deepEqual(classroomCompletion(-1, 0), { ratio: 0, completed: false })

  const values = new Map()
  const storage = {
    getItem: (key) => values.get(key),
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  }
  const anonymous = createClassroomProgressTracker({ contentId: 21, durationSeconds: 100, storage })
  assert.deepEqual(await anonymous.record(25), { positionSeconds: 25, completed: false, local: true })
  assert.deepEqual(await anonymous.record(90), { positionSeconds: 90, completed: true, local: true })
  assert.deepEqual(await anonymous.record(40), { positionSeconds: 40, completed: true, local: true }, 'completed progress must not regress after seeking backward')
  const localProgress = readAnonymousClassroomProgress(storage, 21)
  assert.equal(localProgress.positionSeconds, 40)
  assert.equal(localProgress.completed, true)
  assert.equal(typeof localProgress.updatedAt, 'number')
  assert.equal(anonymous.pending(), null, 'anonymous progress must never queue a server write')

  let now = 1_000
  const sent = []
  let failNext = false
  const loggedIn = createClassroomProgressTracker({
    contentId: 22,
    durationSeconds: 100,
    loggedIn: true,
    now: () => now,
    send: async (contentId, positionSeconds) => {
      if (failNext) { failNext = false; throw new Error('network') }
      sent.push({ contentId, positionSeconds })
      return { positionSeconds }
    },
  })
  await loggedIn.record(10)
  now += 5_000
  await loggedIn.record(20)
  assert.deepEqual(sent, [{ contentId: '22', positionSeconds: 10 }], 'updates inside the window should be throttled')
  assert.deepEqual(loggedIn.pending(), { positionSeconds: 20, completed: false })
  now += 7_000
  await loggedIn.record(30)
  assert.deepEqual(sent.at(-1), { contentId: '22', positionSeconds: 30 })

  now += 1_000
  await loggedIn.record(40)
  await loggedIn.flush()
  assert.deepEqual(sent.at(-1), { contentId: '22', positionSeconds: 40 }, 'pause/unload flush should bypass throttling')

  now += 1_000
  failNext = true
  await assert.rejects(() => loggedIn.record(50, { force: true }), /network/)
  assert.deepEqual(loggedIn.pending(), { positionSeconds: 50, completed: false })
  await loggedIn.retry()
  assert.deepEqual(sent.at(-1), { contentId: '22', positionSeconds: 50 })
  assert.equal(loggedIn.pending(), null)

  function deferred() {
    let resolve
    let reject
    const promise = new Promise((yes, no) => { resolve = yes; reject = no })
    return { promise, resolve, reject }
  }

  const firstSend = deferred()
  const concurrentSent = []
  const concurrent = createClassroomProgressTracker({
    contentId: 23,
    durationSeconds: 100,
    loggedIn: true,
    now: () => now,
    send: async (_contentId, positionSeconds) => {
      concurrentSent.push(positionSeconds)
      if (positionSeconds === 10) await firstSend.promise
    },
  })
  const firstRecord = concurrent.record(10)
  await Promise.resolve()
  await concurrent.record(20)
  await concurrent.record(30)
  assert.deepEqual(concurrentSent, [10], 'records during an in-flight write must not start concurrent requests')
  assert.deepEqual(concurrent.pending(), { positionSeconds: 30, completed: false }, 'only the latest throttled snapshot should remain queued')
  firstSend.resolve()
  await firstRecord
  assert.deepEqual(concurrentSent, [10])
  assert.deepEqual(concurrent.pending(), { positionSeconds: 30, completed: false })

  const flushFirst = deferred()
  const flushSecond = deferred()
  const flushSent = []
  const flushing = createClassroomProgressTracker({
    contentId: 24,
    durationSeconds: 100,
    loggedIn: true,
    now: () => now,
    send: async (_contentId, positionSeconds) => {
      flushSent.push(positionSeconds)
      if (positionSeconds === 10) await flushFirst.promise
      if (positionSeconds === 40) await flushSecond.promise
    },
  })
  const activeRecord = flushing.record(10)
  await Promise.resolve()
  await flushing.record(40)
  let firstFlushDone = false
  let secondFlushDone = false
  const flushPromise = flushing.flush().then(() => { firstFlushDone = true })
  const simultaneousFlush = flushing.flush().then(() => { secondFlushDone = true })
  await Promise.resolve()
  assert.deepEqual(flushSent, [10], 'flush must serialize behind the active request')
  flushFirst.resolve()
  await activeRecord
  await new Promise((resolve) => setTimeout(resolve, 0))
  assert.deepEqual(flushSent, [10, 40], 'flush must send the newest queued snapshot after the active request')
  assert.equal(firstFlushDone || secondFlushDone, false, 'all simultaneous flush callers must wait for the serialized latest write')
  flushSecond.resolve()
  await Promise.all([flushPromise, simultaneousFlush])
  assert.equal(flushing.pending(), null)

  const failingSend = deferred()
  const retrySent = []
  let attempt = 0
  const retrying = createClassroomProgressTracker({
    contentId: 25,
    durationSeconds: 100,
    loggedIn: true,
    now: () => now,
    send: async (_contentId, positionSeconds) => {
      retrySent.push(positionSeconds)
      attempt += 1
      if (attempt === 1) await failingSend.promise
    },
  })
  const failedRecord = retrying.record(10)
  await Promise.resolve()
  await retrying.record(60)
  failingSend.reject(new Error('offline'))
  await assert.rejects(failedRecord, /offline/)
  assert.deepEqual(retrying.pending(), { positionSeconds: 60, completed: false }, 'a failed older request must not replace the newest queued snapshot')
  await retrying.retry()
  assert.deepEqual(retrySent, [10, 60])
  assert.equal(retrying.pending(), null)

  const authoritativeSent = []
  const authoritative = createClassroomProgressTracker({
    contentId: 26,
    durationSeconds: 100,
    loggedIn: true,
    send: async (_contentId, positionSeconds) => {
      authoritativeSent.push(positionSeconds)
      return authoritativeSent.length === 1
        ? { positionSeconds, completed: false }
        : { positionSeconds, completed: true }
    },
  })
  assert.deepEqual(await authoritative.record(95), { positionSeconds: 95, completed: false }, 'logged-in completion must wait for the server')
  assert.deepEqual(await authoritative.record(96, { force: true }), { positionSeconds: 96, completed: true }, 'server completion should update the tracker snapshot')
  assert.deepEqual(await authoritative.record(20), { positionSeconds: 20, completed: true }, 'server-confirmed completion must stay monotonic')

  console.log('classroom progress tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
