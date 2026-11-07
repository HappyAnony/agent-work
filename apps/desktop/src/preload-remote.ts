/** Minimal preload for local pages of a remote-only desktop client; no host filesystem paths are exposed. */
import { contextBridge, ipcRenderer } from 'electron'
import { markDocumentPlatform } from './preload-platform.ts'
import { desktopKeyboardApi } from './preload-keyboard.ts'
import { syncNativeTheme } from './preload-theme.ts'

if (location.protocol === 'dsh-app:' && location.hostname === 'app' && process.isMainFrame) {
  contextBridge.exposeInMainWorld('dshDesktop', { protocolVersion: 1, ...desktopKeyboardApi() })
  contextBridge.exposeInMainWorld('dshDesktopBoot', {
    ready: () => ipcRenderer.invoke('dsh-remote:boot'),
    failed: (message: string) => ipcRenderer.invoke('dsh-remote:failed', message),
  })
}
if (location.protocol === 'dsh-app:' && location.hostname === 'shell' && process.isMainFrame) {
  contextBridge.exposeInMainWorld('agentworkConnect', {
    state: () => ipcRenderer.invoke('dsh-remote:state'),
    connect: (address: string, token: string) => ipcRenderer.invoke('dsh-remote:connect', address, token),
  })
}
markDocumentPlatform()
syncNativeTheme()
