/** Subprocess cancellation owns both the launcher and its descendants under CI overlap. */
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { expect, it, onTestFinished } from 'vitest'
import { temporary } from './fixture.ts'
import { run } from './process.ts'

// Windows has no POSIX process-group signaling; delivery targets are Linux and macOS.
it.skipIf(process.platform === 'win32')('awaits cancellation of the owned descendant before returning', async () => {
  const root = await temporary()
  const ready = join(root, 'ready.json')
  const childSource = 'process.send(process.pid); setInterval(() => {}, 1000)'
  const parentSource = `const {fork}=require('node:child_process'); const fs=require('node:fs');
const child=fork('-e',[${JSON.stringify(childSource)}],{stdio:['ignore','ignore','ignore','ipc']});
child.on('message',pid=>fs.writeFileSync(${JSON.stringify(ready)},JSON.stringify({parent:process.pid,child:pid})));`
  const controller = new AbortController()
  const settled = run(process.execPath, ['-e', parentSource], { cwd: root, timeoutMs: 30_000, signal: controller.signal }, true)
    .then(() => ({ error: undefined }), (error: unknown) => ({ error }))
  onTestFinished(async () => { controller.abort(); await settled })
  await expect.poll(async () => JSON.parse(await readFile(ready, 'utf8')) as { child: number }, { timeout: 15_000 }).toHaveProperty('child')
  const ids = JSON.parse(await readFile(ready, 'utf8')) as { parent: number; child: number }
  controller.abort()
  expect((await settled).error).toMatchObject({ code: 'cancelled' })
  for (const pid of [ids.parent, ids.child]) {
    await expect.poll(() => {
      try { process.kill(pid, 0); return true }
      catch (error) { if ((error as NodeJS.ErrnoException).code === 'ESRCH') return false; throw error }
    }, { timeout: 15_000 }).toBe(false)
  }
})
