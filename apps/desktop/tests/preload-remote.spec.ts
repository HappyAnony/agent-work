// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { DshDesktopProductApi } from '../src/ipc.ts'
import { DESKTOP_IPC } from '../src/ipc.ts'

const electron = vi.hoisted(() => ({
  contextBridge: { exposeInMainWorld: vi.fn<(name: string, value: unknown) => void>() },
  ipcRenderer: { invoke: vi.fn(), on: vi.fn(), off: vi.fn(), send: vi.fn() },
}))
vi.mock('electron', () => electron)
vi.mock('../src/preload-platform.ts', () => ({ markDocumentPlatform: vi.fn() }))
vi.mock('../src/preload-theme.ts', () => ({ syncNativeTheme: vi.fn() }))
beforeEach(() => { vi.stubGlobal('process', { ...process, isMainFrame: true }) })
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); vi.resetModules() })

it('provides native keyboard and boot capabilities while files and directory selection remain remote', async () => {
  vi.stubGlobal('location', new URL('dsh-app://app/'))
  await import('../src/preload-remote.ts')
  const exposed = Object.fromEntries(electron.contextBridge.exposeInMainWorld.mock.calls)
  expect(exposed).toHaveProperty('dshDesktopBoot')
  expect(exposed).not.toHaveProperty('__DSH_HOST_PATHS__')
  expect(exposed).not.toHaveProperty('__DSH_DIRECTORY_PICKER__')
  const desktop = exposed.dshDesktop as Pick<DshDesktopProductApi, 'keyboard' | 'shortcuts'>
  await desktop.shortcuts.get([])
  expect(electron.ipcRenderer.invoke).toHaveBeenCalledWith(DESKTOP_IPC.shortcutsGet, [])
  const listener = vi.fn()
  const dispose = desktop.keyboard.subscribe(listener)
  expect(electron.ipcRenderer.on).toHaveBeenCalledWith(DESKTOP_IPC.shortcutsInput, expect.any(Function))
  dispose()
  expect(electron.ipcRenderer.off).toHaveBeenCalledWith(DESKTOP_IPC.shortcutsInput, expect.any(Function))
})

it.each([{ origin: 'dsh-app://shell/remote.html', main: true, connect: true },
  { origin: 'dsh-app://app/stage/build/portable/', main: false, connect: false },
  { origin: 'https://untrusted.example/', main: true, connect: false }])('limits native capabilities at $origin', async ({ origin, main, connect }) => {
  vi.stubGlobal('location', new URL(origin))
  vi.stubGlobal('process', { ...process, isMainFrame: main })
  await import('../src/preload-remote.ts')
  const names = electron.contextBridge.exposeInMainWorld.mock.calls.map(call => call[0])
  expect(names.includes('agentworkConnect')).toBe(connect)
  expect(names).not.toContain('dshDesktop')
  expect(names).not.toContain('dshDesktopBoot')
})
