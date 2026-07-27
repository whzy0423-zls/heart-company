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
  const localProgress = readAnonymousClassroomProgress(storage, 21)
  assert.equal(localProgress.positionSeconds, 25)
  assert.equal(localProgress.completed, false)
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

  console.log('classroom progress tests passed')
} finally {
  await rm(dir, { force: true, recursive: true })
}
