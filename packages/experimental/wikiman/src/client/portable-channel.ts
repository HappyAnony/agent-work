// SPDX-License-Identifier: AGPL-3.0-or-later
/** Private-port host protocol adapted from IronMan's portable editor bridge. */
import { z } from 'zod'
import { brandString, type Branded } from '@deepseek-ai/dsh-brand'
import type { WikiDocument, WikiEditorView } from '../types.ts'
/** Host-owned tab identity admitted by the scoped editor protocol. */
export type WikiTabId = Branded<'wikiman.tab'>
type DocumentKey = Pick<WikiDocument, 'boxId' | 'docId'>
type EditorView = WikiEditorView['view']
class SessionError extends Error {}
function record(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

const documentSchema = z.object({
  boxId: z.string().regex(/^\d{14}-[a-z0-9]{7}$/),
  docId: z.string().regex(/^\d{14}-[a-z0-9]{7}$/),
})
const commands = [
  'select',
  'close',
  'pin',
  'close-others',
  'close-all',
  'close-left',
  'close-right',
  'split-left',
  'split-right',
  'split-up',
  'split-down',
  'split-copy-right',
  'split-copy-down',
  'move-before',
  'move-after',
] as const
const tabCommands = new Set<string>(commands)
const tabID = /^[a-f0-9-]{36}$/
const tabIdSchema = z.string().regex(tabID).transform(value => brandString<WikiTabId>(value))
const tabIntentSchema = z.object({
  command: z.enum(commands),
  tabId: tabIdSchema,
  targetId: tabIdSchema.optional(),
})
const pluginActionsSchema = z
  .array(
    z.object({
      id: z.string().regex(/^p[0-7]-[0-9]{1,2}$/),
      label: z.string().min(1).max(96),
      region: z.enum(['left', 'right', 'bottom', 'status']),
    }),
  )
  .max(96)
/** One page capability and initial document transferred on a private port. */
export interface FrameMount {
  baseUrl: string
  streamBaseUrl?: string | undefined
  view: EditorView
  document: DocumentKey | null
  focus?: { blockId: string; hash: string }
  hostPluginLayout?: boolean
  locale?: string
}
/** A label and region advertised by a scoped Wiki app plugin. */
export interface PluginHostAction {
  id: string
  label: string
  region: 'left' | 'right' | 'bottom' | 'status'
}
/** Tab and split operations understood by the embedded Wiki app. */
export type TabCommand =
  | 'select'
  | 'close'
  | 'pin'
  | 'close-others'
  | 'close-all'
  | 'close-left'
  | 'close-right'
  | 'split-left'
  | 'split-right'
  | 'split-up'
  | 'split-down'
  | 'split-copy-right'
  | 'split-copy-down'
  | 'move-before'
  | 'move-after'
/** An embedded tab action referring to host-owned tab identities. */
export interface TabIntent {
  command: TabCommand
  tabId: WikiTabId
  targetId?: WikiTabId | undefined
}
/** Visible tabs, focus and sidebar state for one pane. */
export interface TabPresentation {
  tabs: { id: WikiTabId; title: string; pinned: boolean; dirty: boolean }[]
  active: WikiTabId
  sidebar: boolean
  focused: boolean
}
/** Device-local split layout and document locations without credentials. */
export type SavedTabTree =
  | { tabs: (DocumentKey & { pinned: boolean })[]; active: number }
  | {
    direction: 'horizontal' | 'vertical'
    ratio: number
    children: [SavedTabTree, SavedTabTree]
  }
/** Versioned device-local layout restored under fresh view capabilities. */
export interface SavedTabs {
  version: 1
  root: SavedTabTree | null
  focused: number
}
type FrameRequest = 'flush' | 'dispose' | 'focus' | 'history' | 'switch' | 'activate' | 'saveTabs'
const responseType = (request: unknown) =>
  request === 'history'
    ? 'historyResult'
    : request === 'focus'
      ? 'focusResult'
      : request === 'switch' || request === 'activate' || request === 'saveTabs'
        ? 'tabResult'
        : 'result'
const drainsInput = (request: unknown) => request === 'flush' || request === 'dispose'
interface Identity {
  channelId: string
  storeId: string
  pageId: string
  sessionId: string
}
function identity(channelId: string, view: EditorView): Identity {
  return {
    channelId,
    storeId: view.storeId,
    pageId: view.pageId,
    sessionId: view.sessionId,
  }
}
function matches(value: Record<string, unknown>, expected: Identity): boolean {
  return Object.entries(expected).every(([key, item]) => value[key] === item)
}

/** Commands accepted only after the embedded editor has mounted. */
export interface PortableChannel {
  readonly ready: Promise<void>
  flush(): Promise<void>
  dispose(): Promise<void>
  focus(focus?: FrameMount['focus']): Promise<void>
  history(redo: boolean): Promise<boolean>
  close(): void
  prepareTabSwitch(): Promise<void>
  activateTab(): Promise<void>
  saveTabs(layout: SavedTabs): Promise<void>
  presentTabs(value: TabPresentation): void
  pluginHostAction(id: string): void
  pluginSystemShortcut(chord: string): void
}
/** Host half for an already loaded controlled iframe. Every reply is bound to
 * the private channel and full source identity; no ambient window messages can
 * acknowledge a save/release. UI mounting/source switching stays host-owned.
 * @param options - verified iframe identity, page capability and host callbacks.
 * @returns commands and readiness for this private editor channel.
 */
export function connectEditorFrame(options: {
  frameWindow: Window
  frameOrigin: string
  channelId: string
  payload: FrameMount
  dirty(dirty: boolean): void
  timeoutMs?: number
  navigate?(blockId: string, reuseCurrent?: boolean): Promise<void>
  tabIntent?(intent: TabIntent): Promise<void>
  tabTitle?(title: string): void
  treeDigest?(digest: string): void
  tabFocus?(): void
  savedTabs?(value: unknown): void
  pluginSystemShortcuts?(chords: string[]): void
  pluginHostActions?(actions: PluginHostAction[]): void
}): PortableChannel {
  if (options.frameOrigin !== (options.payload.baseUrl.startsWith('dsh-app://app/') ? 'dsh-app://app' : new URL(options.payload.baseUrl).origin)) {
    throw new SessionError('invalid_frame_origin')
  }
  const channel = new MessageChannel(),
    expected = identity(options.channelId, options.payload.view)
  const pending = new Map<
    string,
    {
      type: FrameRequest
      resolve(handled: boolean): void
      reject(error: unknown): void
      timer: ReturnType<typeof setTimeout>
    }
  >()
  let closed = false,
    resolveReady!: () => void,
    rejectReady!: (error: unknown) => void
  let mounted = false,
    navigating = false,
    commandSequence = 0
  const seenNavigation = new Set<string>()
  const ready = new Promise<void>((resolve, reject) => {
    resolveReady = resolve
    rejectReady = reject
  })
  const readyTimer = setTimeout(() => {
    rejectReady(new SessionError('editor_mount_timeout'))
    close()
  }, options.timeoutMs ?? 15000)
  const close = () => {
    if (closed) {
      return
    }
    closed = true
    clearTimeout(readyTimer)
    channel.port1.close()
    rejectReady(new SessionError('editor_channel_closed'))
    for (const wait of pending.values()) {
      clearTimeout(wait.timer)
      wait.reject(new SessionError('editor_channel_closed'))
    }
    pending.clear()
  }
  channel.port1.onmessage = (message: MessageEvent) => {
    const reply: unknown = message.data
    if (!record(reply) || !matches(reply, expected) || closed) {
      return
    }
    if (reply.type === 'ready') {
      if (mounted) {
        return
      }
      mounted = true
      options.savedTabs?.(reply.savedTabs)
      clearTimeout(readyTimer)
      resolveReady()
      return
    }
    if (
      reply.type === 'treeDigest' &&
      mounted &&
      typeof reply.digest === 'string' &&
      /^[a-f0-9]{64}$/.test(reply.digest)
    ) {
      options.treeDigest?.(reply.digest)
      return
    }
    if (reply.type === 'tabFocus' && mounted) {
      options.tabFocus?.()
      return
    }
    if (reply.type === 'pluginHostActions' && mounted && options.payload.hostPluginLayout) {
      const parsed = pluginActionsSchema.safeParse(reply.actions)
      if (parsed.success) {
        options.pluginHostActions?.(parsed.data)
      }
      return
    }
    if (
      reply.type === 'pluginSystemShortcuts' &&
      mounted &&
      options.payload.view.mode === 'wikiDesktop' &&
      Array.isArray(reply.chords) &&
      reply.chords.length <= 24 &&
      reply.chords.every(chord => typeof chord === 'string' && /^⌥⇧⌘[A-Z0-9]$/.test(chord))
    ) {
      options.pluginSystemShortcuts?.(reply.chords.filter((chord): chord is string => typeof chord === 'string'))
      return
    }
    if (
      reply.type === 'tabTitle' &&
      options.payload.view.mode === 'wikiDesktop' &&
      typeof reply.title === 'string' &&
      reply.title.length > 0 &&
      reply.title.length <= 4096
    ) {
      options.tabTitle?.(reply.title)
      return
    }
    if (reply.type === 'tabIntent' && mounted && options.payload.view.mode === 'wikiDesktop' && options.tabIntent) {
      const intent = tabIntentSchema.safeParse(reply.intent).data
      if (
        !intent ||
        !tabCommands.has(intent.command) ||
        typeof intent.tabId !== 'string' ||
        !tabID.test(intent.tabId) ||
        (intent.targetId !== undefined && (typeof intent.targetId !== 'string' || !tabID.test(intent.targetId)))
      ) {
        return
      }
      void options.tabIntent(intent).catch((error: unknown) => {
        if (!closed) {
          channel.port1.postMessage({
            type: 'tabError',
            code: error instanceof Error ? error.message : 'editor_command_failed',
            ...expected,
          })
        }
      })
      return
    }
    if (reply.type === 'navigate') {
      const doc = documentSchema.nullable().optional().safeParse(reply.document).data,
        id = reply.requestId
      const owner = options.payload.document
      const sameDocument = owner === null ? doc === null : doc?.boxId === owner.boxId && doc.docId === owner.docId
      if (
        !mounted ||
        typeof id !== 'string' ||
        !id ||
        id.length > 128 ||
        seenNavigation.has(id) ||
        seenNavigation.size >= 1024 ||
        !sameDocument ||
        typeof reply.blockId !== 'string' ||
        !/^\d{14}-[a-z0-9]{7}$/.test(reply.blockId)
      ) {
        return
      }
      seenNavigation.add(id)
      const respond = (ok: boolean, code?: string) => {
        if (!closed) {
          channel.port1.postMessage({
            type: 'navigationResult',
            requestId: id,
            ok,
            code,
            ...expected,
          })
        }
      }
      if (navigating || !options.navigate) {
        respond(false, navigating ? 'reference_navigation_busy' : 'reference_navigation_unavailable')
        return
      }
      const navigate = options.navigate.bind(options)
      navigating = true
      const blockId = reply.blockId
      void Promise.resolve()
        .then(() =>
          reply.reuseCurrent === true && options.payload.view.mode === 'wikiDesktop'
            ? navigate(blockId, true)
            : navigate(blockId),
        )
        .then(
          () =>{  respond(true) },
          (error: unknown) => {  respond(false, error instanceof Error ? error.message : 'reference_navigation_failed') },
        )
        .finally(() => {
          navigating = false
        })
      return
    }
    if (reply.type === 'dirty' && typeof reply.dirty === 'boolean') {
      options.dirty(reply.dirty)
      return
    }
    if (reply.type === 'error') {
      options.dirty(true)
      rejectReady(new SessionError(String(reply.code)))
      close()
      return
    }
    if (
      !['result', 'focusResult', 'historyResult', 'tabResult'].includes(String(reply.type)) ||
      typeof reply.requestId !== 'string'
    ) {
      return
    }
    const wait = pending.get(reply.requestId)
    if (!wait) {
      return
    }
    if (responseType(wait.type) !== reply.type) {
      return
    }
    pending.delete(reply.requestId)
    clearTimeout(wait.timer)
    if (reply.ok === true && (!drainsInput(wait.type) || reply.dirty === false)) {
      wait.resolve(reply.handled === true)
    } else {
      if (drainsInput(wait.type)) {
        options.dirty(true)
      }
      wait.reject(new SessionError(typeof reply.code === 'string' ? reply.code : 'unsaved_editor_state'))
    }
  }
  channel.port1.start()
  options.frameWindow.postMessage(
    {
      type: 'wiki:editor:init',
      channelId: options.channelId,
      payload: options.payload,
    },
    options.frameOrigin,
    [channel.port2],
  )
  const request = async (
    type: FrameRequest,
    data?: { redo?: boolean; focus?: FrameMount['focus']; layout?: SavedTabs },
  ) => {
    await ready
    if (closed) {
      throw new SessionError('editor_channel_closed')
    }
    const requestId = `command-${++commandSequence}`
    return new Promise<boolean>((resolve, reject) => {
      const timer = setTimeout(() => {
        pending.delete(requestId)
        if (drainsInput(type)) {
          options.dirty(true)
        }
        reject(
          new SessionError(
            type === 'focus'
              ? 'reference_focus_timeout'
              : type === 'history'
                ? 'editor_history_timeout'
                : 'editor_flush_timeout',
          ),
        )
      }, options.timeoutMs ?? 15000)
      pending.set(requestId, { type, resolve, reject, timer })
      channel.port1.postMessage({ type, requestId, ...data, ...expected })
    })
  }
  return {
    pluginHostAction(id: string) {
      if (mounted && !closed) {
        channel.port1.postMessage({ type: 'pluginHostAction', id, ...expected })
      }
    },
    pluginSystemShortcut(chord: string) {
      if (mounted && !closed) {
        channel.port1.postMessage({
          type: 'pluginSystemShortcut',
          chord,
          ...expected,
        })
      }
    },
    ready,
    flush: async () => {
      await request('flush')
    },
    dispose: async () => {
      await request('dispose')
      close()
    },
    focus: async (focus?: FrameMount['focus']) => {
      await request('focus', { focus })
    },
    history: (redo: boolean) => request('history', { redo }),
    close,
    prepareTabSwitch: async () => {
      await request('switch')
    },
    activateTab: async () => {
      await request('activate')
    },
    saveTabs: async (layout: SavedTabs) => {
      await request('saveTabs', { layout })
    },
    presentTabs: (value: TabPresentation) => {
      if (mounted && !closed) {
        channel.port1.postMessage({ type: 'tabs', value, ...expected })
      }
    },
  }
}

/** Validate saved layout limits before issuing additional editor capabilities.
 * @param value - layout from the embedded app's device-local preferences.
 * @returns a bounded layout, or undefined for unavailable or invalid preferences.
 */
export function parseSavedTabs(value: unknown): SavedTabs | undefined {
  let tabs = 0
  const tree = (node: unknown, depth: number): node is SavedTabTree => {
    if (!record(node) || depth > 8) return false
    if ('children' in node)
      return (
        (node.direction === 'horizontal' || node.direction === 'vertical') &&
        typeof node.ratio === 'number' &&
        node.ratio >= 15 &&
        node.ratio <= 85 &&
        Array.isArray(node.children) &&
        node.children.length === 2 &&
        node.children.every(child => tree(child, depth + 1))
      )
    if (
      !Array.isArray(node.tabs) ||
      !node.tabs.length ||
      typeof node.active !== 'number' ||
      !Number.isInteger(node.active) ||
      node.active < 0 ||
      node.active >= node.tabs.length
    )
      return false
    tabs += node.tabs.length
    return (
      tabs <= 32 && node.tabs.every(item => documentSchema.extend({ pinned: z.boolean() }).safeParse(item).success)
    )
  }
  if (
    !record(value) ||
    value.version !== 1 ||
    typeof value.focused !== 'number' ||
    !Number.isInteger(value.focused) ||
    value.focused < 0 ||
    value.focused >= 32
  )
    return
  if (value.root !== null && !tree(value.root, 0)) return
  return { version: 1, focused: value.focused, root: value.root }
}
