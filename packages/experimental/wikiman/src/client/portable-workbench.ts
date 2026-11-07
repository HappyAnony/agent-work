// SPDX-License-Identifier: AGPL-3.0-or-later
/** Tab and split ownership adapted from IronMan's portable workbench host. */
import { randomUUID } from '@deepseek-ai/dsh-util-crypto'
import { brandString } from '@deepseek-ai/dsh-brand'
import type { WikiTabId } from './portable-channel.ts'
import {
  connectEditorFrame,
  type FrameMount,
  type PluginHostAction,
  type SavedTabs,
  type SavedTabTree,
  type TabIntent,
  parseSavedTabs,
} from './portable-channel.ts'
import type { WikiDocument, WikiEditorView, WikiLink } from '../types.ts'
function record(value:unknown):value is Record<string,unknown>{return typeof value==='object'&&value!==null}
function present<T>(value:T|null|undefined):T {if(value===null||value===undefined)throw new Error('editor_state_unavailable');return value}
type DocumentKey = Pick<WikiDocument, 'boxId' | 'docId'>
type SharedEditorView = WikiEditorView['view']
interface ReferenceLocation extends WikiDocument {
  blockId: string
  mode: 'wikiDesktop' | 'docxSingle'
}
/** Kernel generation and page selected in the Wiki workbench. */
export interface PortableTarget {
  baseUrl: string
  storeId: string
  sessionId: string
  pageId: string
  mode: 'wikiDesktop' | 'docxSingle'
}
/** Changes the selected page when a native reference crosses Wiki pages. */
export interface ReferenceRouter {
  current(origin: PortableTarget): boolean
  select(location: ReferenceLocation, origin: PortableTarget): void
}
/** DSH-owned capability, document and localization operations. */
export interface PortableEditorHost {
  replaceSystemShortcuts?(view: SharedEditorView | null, chords: string[]): Promise<void>
  openEditor(
    storeId: string,
    sessionId: string,
    pageId: string,
    mode: PortableTarget['mode'],
  ): Promise<SharedEditorView>
  closeEditor(view: SharedEditorView): Promise<unknown>
  reportEditorDirty(view: SharedEditorView, dirty: boolean): Promise<void>
  documents(target: PortableTarget, cursor: string): Promise<{ items: readonly WikiDocument[]; nextCursor: string }>
  resolve(target: PortableTarget, blockId: string): Promise<WikiLink | null>
  locale(): string
  label(kind: 'editor' | 'resize'): string
}
type DocumentItem = WikiDocument
interface Snapshot {
  phase: 'idle' | 'opening' | 'ready' | 'error'
  target: PortableTarget | null
  documents: DocumentItem[]
  selected: DocumentKey | null
  dirty: boolean
  error: string | null
  pluginActions?: PluginHostAction[] | undefined
  pluginViewId?: string | undefined
}
interface Entry {
  id: WikiTabId
  target: PortableTarget
  view: SharedEditorView
  frame: HTMLIFrameElement
  connection: ReturnType<typeof connectEditorFrame> | null
  reports: Promise<void>
  disposed: boolean
  ready: boolean
  document: DocumentKey | null
  documents: DocumentItem[]
  title: string
  pinned: boolean
  dirty: boolean
  savedTabs?: SavedTabs
  systemShortcuts?: string[]
  pluginActions?: PluginHostAction[]
}
interface Pane {
  kind: 'pane'
  tabs: Entry[]
  active: Entry
}
interface Split {
  kind: 'split'
  direction: 'horizontal' | 'vertical'
  first: Layout
  second: Layout
  ratio: number
  divider: HTMLDivElement
}
type Layout = Pane | Split
type Rect = { x: number; y: number; width: number; height: number }
const same = (a: PortableTarget, b: PortableTarget) =>
  a.baseUrl === b.baseUrl &&
  a.storeId === b.storeId &&
  a.sessionId === b.sessionId &&
  a.pageId === b.pageId &&
  a.mode === b.mode
const sameDoc = (a: DocumentKey, b: DocumentKey) => a.boxId === b.boxId && a.docId === b.docId
const message = (error: unknown) => (error instanceof Error ? error.message : 'wiki_editor_operation_failed')

/** One owner outside React's mount lifetime. Hidden views are parked, never
 * destroyed by an effect cleanup while input/IME is still unconfirmed. Every
 * transition is serialized, and a stale open can only close its own ticket. */
export class PortableEditor {
  private entry: Entry | null = null
  private entries = new Set<Entry>()
  private layout: Layout | null = null
  private tail: Promise<unknown> = Promise.resolve()
  private epoch = 0
  private pendingReference: {
    target: PortableTarget
    location: ReferenceLocation
  } | null = null
  private listeners = new Set<() => void>()
  private holder = document.createElement('div')
  private anchor: HTMLElement | null = null
  private shortcutUpdate = Promise.resolve()
  private shortcutIdentity = ''
  private shortcutModuleActive = true
  /** Enable host shortcut delivery while the Wiki module is active.
   * @param active - whether the Wiki panel owns host shortcuts.
   */
  setShortcutModuleActive(active: boolean): void {
    this.shortcutModuleActive = active
    this.refreshShortcuts()
    this.publishPluginActions()
  }
  private publishPluginActions() {
    const actions =
      this.shortcutModuleActive && this.anchor?.isConnected && !this.entry?.disposed
        ? this.entry?.pluginActions
        : undefined
    const viewId = actions?.length ? this.entry?.view.viewId : undefined
    if (this.snapshot.pluginActions !== actions || this.snapshot.pluginViewId !== viewId) {
      this.publish({ pluginActions: actions, pluginViewId: viewId })
    }
  }
  /** Dispatch a currently advertised app-plugin dock action.
   * @param id - action identity advertised by the current editor.
   * @param viewId - view that advertised this action.
   */
  runPluginHostAction(id: string, viewId: string): void {
    const entry = this.entry
    if (
      this.shortcutModuleActive &&
      this.anchor?.isConnected &&
      entry?.view.viewId === viewId &&
      !entry.disposed &&
      entry.pluginActions?.some(action => action.id === id)
    ) {
      entry.connection?.pluginHostAction(id)
    }
  }
  private refreshShortcuts() {
    const replace = this.host.replaceSystemShortcuts?.bind(this.host)
    if (!replace) {
      return
    }
    const entry = this.shortcutModuleActive && this.anchor?.isConnected && !this.entry?.disposed ? this.entry : null
    const chords = entry?.systemShortcuts ?? []
    const next = entry && chords.length ? entry.view.viewId + ':' + chords.join(',') : ''
    if (next === this.shortcutIdentity) {
      return
    }
    this.shortcutIdentity = next
    this.shortcutUpdate = this.shortcutUpdate
      .then(() => replace(entry && chords.length ? entry.view : null, chords))
      .catch(() => {
        if (this.shortcutIdentity === next) {
          this.shortcutIdentity = ''
        }
      })
  }
  /** Deliver an advertised shortcut to its active iframe.
   * @param viewId - view owning the shortcut.
   * @param chord - chord registered by the app plugin.
   */
  handleSystemShortcut(viewId: string, chord: string): void {
    const entry = this.entry
    if (
      this.shortcutModuleActive &&
      entry?.view.viewId === viewId &&
      this.anchor?.isConnected &&
      !entry.disposed &&
      entry.systemShortcuts?.includes(chord)
    ) {
      entry.connection?.pluginSystemShortcut(chord)
    }
  }
  private resizeObserver: ResizeObserver | null = null
  private snapshot: Snapshot = {
    phase: 'idle',
    target: null,
    documents: [],
    selected: null,
    dirty: false,
    error: null,
  }
  constructor(
    private host: PortableEditorHost,
    private connect = connectEditorFrame,
    private router?: ReferenceRouter,
    private onTreeDigest?: (target: PortableTarget, digest: string) => void,
  ) {
    this.holder.id = 'wiki-portable-owner-' + randomUUID()
    this.holder.style.cssText = 'position:fixed;visibility:hidden;z-index:10'
    window.addEventListener('resize', this.position)
    window.addEventListener('scroll', this.position, true)
    if (typeof ResizeObserver !== 'undefined') {
      this.resizeObserver = new ResizeObserver(this.position)
    }
  }
  private position = () => {
    if (!this.anchor?.isConnected) {
      this.holder.style.display = 'none'
      return
    }
    const rect = this.anchor.getBoundingClientRect()
    Object.assign(this.holder.style, {
      display: 'block',
      visibility: 'visible',
      left: String(rect.left) + 'px',
      top: String(rect.top) + 'px',
      width: String(rect.width) + 'px',
      height: String(rect.height) + 'px',
    })
  }
  /** Read the immutable workbench state for a React subscription.
   * @returns current selection, save status and operation error.
   */
  getSnapshot = () => this.snapshot
  /** Subscribe to workbench state updates.
   * @param listener - notified after the current state changes.
   * @returns the subscription disposer.
   */
  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => {
      this.listeners.delete(listener)
    }
  }
  private publish(next: Partial<Snapshot>) {
    this.snapshot = { ...this.snapshot, ...next }
    for (const listener of this.listeners) {
      listener()
    }
  }
  private serialize<T>(work: () => Promise<T>): Promise<T> {
    const result = this.tail.then(work)
    this.tail = result.catch(() => undefined)
    return result
  }
  /** Position stable iframe owners over the panel's editor seat.
   * @param container - visible seat whose size and position should be followed.
   */
  attach(container: HTMLElement): void {
    // Reparenting an iframe reloads its browsing context even when its element
    // is retained. Keep one stable body-level owner; only move its viewport.
    this.resizeObserver?.disconnect()
    this.anchor = container
    container.dataset.editorOwner = this.holder.id
    if (!this.holder.isConnected) {
      document.body.appendChild(this.holder)
    }
    this.resizeObserver?.observe(container)
    this.position()
    this.refreshShortcuts()
    this.publishPluginActions()
  }
  /** Hide editor frames without moving them or losing pending input. */
  park(): void {
    this.anchor = null
    this.resizeObserver?.disconnect()
    this.holder.style.display = 'none'
    this.refreshShortcuts()
    this.publishPluginActions()
  }
  private panes(layout = this.layout): Pane[] {
    if (!layout) {
      return []
    }
    return layout.kind === 'pane' ? [layout] : [...this.panes(layout.first), ...this.panes(layout.second)]
  }
  private paneOf(entry: Entry) {
    return this.panes().find(pane => pane.tabs.includes(entry))
  }
  private visible(entry: Entry) {
    return this.paneOf(entry)?.active === entry && !entry.disposed
  }
  private publishEntry(entry: Entry | null) {
    this.entry = entry
    this.refreshShortcuts()
    this.publishPluginActions()
    this.publish({
      phase: 'ready',
      target: entry?.target ?? this.snapshot.target,
      selected: entry?.document ?? null,
      documents: entry?.documents ?? [],
      dirty: this.isDirty(),
      error: null,
    })
  }
  private isDirty() {
    return Array.from(this.entries).some(entry => entry.dirty)
  }
  private async saveLayout() {
    const owner = this.entry
    if (!owner?.ready || owner.disposed || owner.target.mode !== 'wikiDesktop' || !this.layout) {
      return
    }
    const encode = (layout: Layout): SavedTabTree | null => {
      if (layout.kind === 'pane') {
        const live = layout.tabs.filter(tab => tab.document && !tab.disposed)
        return live.length
          ? {
            tabs: live.map(tab => ({
              ...present(tab.document),
              pinned: tab.pinned,
            })),
            active: Math.max(0, live.indexOf(layout.active)),
          }
          : null
      }
      const first = encode(layout.first),
        second = encode(layout.second)
      return first && second
        ? {
          direction: layout.direction,
          ratio: Math.round(layout.ratio * 100),
          children: [first, second],
        }
        : (first ?? second)
    }
    const value: SavedTabs = {
      version: 1,
      root: encode(this.layout),
      focused: Math.max(
        0,
        this.panes().findIndex(pane => pane.active === owner),
      ),
    }
    try {
      await present(owner.connection).saveTabs(value)
    } catch {
      if (this.entry === owner) {
        this.publish({ error: 'editor_layout_save_failed' })
      }
    }
  }
  private async restoreLayout(initial: Entry, epoch: number, explicit: boolean): Promise<Entry> {
    const state = initial.savedTabs
    if (!state || initial.target.mode !== 'wikiDesktop') {
      return initial
    }
    let initialUsed = false
    const restoredInitial = (): boolean => initialUsed
    const initialPinned = initial.pinned
    const restored: Entry[] = []
    const dividers: HTMLElement[] = []
    const panes: Pane[] = []
    const build = async (node: SavedTabTree): Promise<Layout | null> => {
      if ('children' in node) {
        const first = await build(node.children[0]),
          second = await build(node.children[1])
        if (!first || !second) {
          return first ?? second
        }
        const split = this.createSplit(node.direction, first, second)
        split.ratio = node.ratio / 100
        dividers.push(split.divider)
        return split
      }
      const tabs: Entry[] = []
      let active: Entry | undefined
      for (const [index, record] of node.tabs.entries()) {
        if (epoch !== this.epoch) {
          throw new Error('stale_editor_layout')
        }
        let tab: Entry | undefined
        if (!initialUsed && initial.document && sameDoc(record, initial.document)) {
          initialUsed = true
          tab = initial
        } else {
          if (explicit && !initialUsed && restored.length === 31) {
            continue
          }
          try {
            tab = await this.mount(initial.target, record, undefined, epoch)
          } catch (error) {
            if (message(error) === 'document_not_in_view') {
              continue
            }
            throw error
          }
          if (tab) {
            restored.push(tab)
          }
        }
        if (!tab) {
          continue
        }
        tab.pinned = record.pinned
        tabs.push(tab)
        if (index === node.active) {
          active = tab
        }
      }
      if (!tabs.length) {
        return null
      }
      const pane: Pane = { kind: 'pane', tabs, active: active ?? present(tabs[0]) }
      panes.push(pane)
      return pane
    }
    try {
      const layout = state.root ? await build(state.root) : null
      if (epoch !== this.epoch) {
        throw new Error('stale_editor_layout')
      }
      if (!layout) {
        if (state.root || explicit || !initial.document) {
          return initial
        }
        const empty = await this.mount(initial.target, undefined, undefined, epoch, true)
        if (!empty) {
          return initial
        }
        await this.closeEntry(initial)
        return empty
      }
      let pane = panes[Math.min(state.focused, panes.length - 1)]
      if (!pane) {
        throw new Error('editor_layout_restore_failed')
      }
      if (explicit) {
        const containing = panes.find(item => item.tabs.includes(initial))
        if (containing) {
          pane = containing
        } else {
          pane.tabs.push(initial)
        }
        pane.active = initial
      } else if (!restoredInitial()) {
        await this.closeEntry(initial)
      }
      this.layout = layout
      return pane.active
    } catch (error) {
      initial.pinned = initialPinned
      for (const entry of restored) {
        await this.closeEntry(entry)
      }
      dividers.forEach((divider) =>{  divider.remove() })
      if (epoch !== this.epoch) {
        throw error
      }
      this.publish({ error: 'editor_layout_restore_failed' })
      return initial
    }
  }
  private renderLayout() {
    const panes = this.panes()
    for (const entry of this.entries) {
      entry.frame.style.visibility = this.visible(entry) ? 'visible' : 'hidden'
    }
    const render = (layout: Layout, rect: Rect) => {
      if (layout.kind === 'pane') {
        const current = layout.active
        Object.assign(current.frame.style, {
          left: String(rect.x) + '%',
          top: String(rect.y) + '%',
          width: String(rect.width) + '%',
          height: String(rect.height) + '%',
        })
        current.connection?.presentTabs({
          active: current.id,
          sidebar: layout === panes[0],
          focused: current === this.entry,
          tabs: layout.tabs
            .filter(tab => !!tab.document && !tab.disposed)
            .map(({ id, title, pinned, dirty }) => ({
              id,
              title,
              pinned,
              dirty,
            })),
        })
        return
      }
      const horizontal = layout.direction === 'horizontal'
      const first = horizontal
        ? { ...rect, width: rect.width * layout.ratio }
        : { ...rect, height: rect.height * layout.ratio }
      const second = horizontal
        ? { ...rect, x: rect.x + first.width, width: rect.width - first.width }
        : {
          ...rect,
          y: rect.y + first.height,
          height: rect.height - first.height,
        }
      Object.assign(
        layout.divider.style,
        horizontal
          ? {
            left: String(second.x) + '%',
            top: String(rect.y) + '%',
            height: String(rect.height) + '%',
            width: '8px',
            transform: 'translateX(-50%)',
            cursor: 'col-resize',
          }
          : {
            left: String(rect.x) + '%',
            top: String(second.y) + '%',
            width: String(rect.width) + '%',
            height: '8px',
            transform: 'translateY(-50%)',
            cursor: 'row-resize',
          },
      )
      layout.divider.setAttribute('aria-valuenow', String(Math.round(layout.ratio * 100)))
      render(layout.first, first)
      render(layout.second, second)
    }
    if (this.layout) {
      render(this.layout, { x: 0, y: 0, width: 100, height: 100 })
    }
  }
  private replacePane(layout: Layout, pane: Pane, replacement: Layout): Layout {
    if (layout === pane) {
      return replacement
    }
    if (layout.kind === 'split') {
      layout.first = this.replacePane(layout.first, pane, replacement)
      layout.second = this.replacePane(layout.second, pane, replacement)
    }
    return layout
  }
  private prune(layout: Layout | null): Layout | null {
    if (!layout || layout.kind === 'pane') {
      return layout?.tabs.length ? layout : null
    }
    const first = this.prune(layout.first),
      second = this.prune(layout.second)
    if (!first || !second) {
      layout.divider.remove()
      return first ?? second
    }
    layout.first = first
    layout.second = second
    return layout
  }
  private createSplit(direction: Split['direction'], first: Layout, second: Layout): Split {
    const divider = document.createElement('div')
    divider.style.cssText = 'position:absolute;z-index:2;background:var(--dsw-alias-border-l4);touch-action:none'
    divider.setAttribute('role', 'separator')
    divider.setAttribute('aria-label', this.host.label('resize'))
    divider.setAttribute('aria-orientation', direction === 'horizontal' ? 'vertical' : 'horizontal')
    divider.setAttribute('aria-valuemin', '15')
    divider.setAttribute('aria-valuemax', '85')
    divider.tabIndex = 0
    const split: Split = {
      kind: 'split',
      direction,
      first,
      second,
      ratio: 0.5,
      divider,
    }
    const resize = (ratio: number) => {
      split.ratio = Math.max(0.15, Math.min(0.85, ratio))
      this.renderLayout()
    }
    const axis = direction === 'horizontal' ? 'clientX' : 'clientY'
    divider.addEventListener('keydown', (event) => {
      const step = (direction === 'horizontal' ? ['ArrowLeft', 'ArrowRight'] : ['ArrowUp', 'ArrowDown']).indexOf(
        event.key,
      )
      if (step < 0) {
        return
      }
      event.preventDefault()
      resize(split.ratio + (step === 0 ? -0.05 : 0.05))
      void this.serialize(() => this.saveLayout())
    })
    divider.addEventListener('pointerdown', (event) => {
      if (event.button !== 0) {
        return
      }
      const measure = (layout: Layout): DOMRect =>
        layout.kind === 'pane' ? layout.active.frame.getBoundingClientRect() : measure(layout.first)
      const outer = direction === 'horizontal' ? measure(split.first).left : measure(split.first).top
      const far = (layout: Layout): DOMRect =>
        layout.kind === 'pane' ? layout.active.frame.getBoundingClientRect() : far(layout.second)
      const total = (direction === 'horizontal' ? far(split.second).right : far(split.second).bottom) - outer
      if (total <= 0) {
        return
      }
      divider.setPointerCapture(event.pointerId)
      const move = (move: PointerEvent) =>{  resize((move[axis] - outer) / total) }
      divider.addEventListener('pointermove', move)
      divider.addEventListener(
        'lostpointercapture',
        () => {
          divider.removeEventListener('pointermove', move)
          void this.serialize(() => this.saveLayout())
        },
        { once: true },
      )
    })
    this.holder.appendChild(divider)
    return split
  }
  private tabIntent(origin: Entry, intent: TabIntent): Promise<void> {
    const epoch = this.epoch
    return this.serialize(async () => {
      const pane = this.paneOf(origin),
        tab = Array.from(this.entries).find(entry => entry.id === intent.tabId)
      if (
        epoch !== this.epoch ||
        !this.anchor?.isConnected ||
        !this.visible(origin) ||
        !pane ||
        !tab ||
        !tab.document ||
        !same(tab.target, origin.target) ||
        (this.router && !this.router.current(origin.target))
      ) {
        throw new Error('stale_editor_tab')
      }
      const from = this.paneOf(tab)
      if (!from) {
        throw new Error('stale_editor_tab')
      }
      const { command } = intent
      const target = pane.tabs.find(item => item.id === intent.targetId)
      const move = command === 'move-before' || command === 'move-after'
      const copy = command === 'split-copy-right' || command === 'split-copy-down'
      const split = command.startsWith('split-') && !copy
      if (from !== pane && !move && !split) {
        throw new Error('stale_editor_tab')
      }
      if (command === 'select') {
        if (tab !== origin) {
          await present(origin.connection).prepareTabSwitch()
        }
        if (epoch !== this.epoch) {
          return
        }
        pane.active = tab
        this.publishEntry(tab)
        this.renderLayout()
        await this.focusEntry(tab)
      } else if (command === 'pin') {
        tab.pinned = !tab.pinned
        pane.tabs.sort((a, b) => Number(b.pinned) - Number(a.pinned))
        this.renderLayout()
      } else if (copy) {
        if (this.entries.size >= 32) {
          throw new Error('editor_tab_limit')
        }
        await present(pane.active.connection).prepareTabSwitch()
        const duplicate = await this.mount(tab.target, tab.document, undefined, epoch)
        if (!duplicate) {
          return
        }
        if (epoch !== this.epoch) {
          await this.closeEntry(duplicate)
          return
        }
        const sibling: Pane = {
          kind: 'pane',
          tabs: [duplicate],
          active: duplicate,
        }
        this.layout = this.replacePane(
          present(this.layout),
          pane,
          this.createSplit(command === 'split-copy-right' ? 'horizontal' : 'vertical', pane, sibling),
        )
        this.publishEntry(duplicate)
        this.renderLayout()
        await this.focusEntry(duplicate)
      } else if (move || split) {
        if (
          (move && (!target || target === tab || target.pinned !== tab.pinned)) ||
          (split && from === pane && from.tabs.length === 1)
        ) {
          return
        }
        await present(pane.active.connection).prepareTabSwitch()
        if (from !== pane && from.active === tab) {
          await present(tab.connection).prepareTabSwitch()
        }
        if (epoch !== this.epoch) {
          return
        }
        this.removeTab(from, tab)
        if (move) {
          const index = pane.tabs.indexOf(present(target))
          pane.tabs.splice(index + (command === 'move-after' ? 1 : 0), 0, tab)
          pane.active = tab
        } else {
          const next: Pane = { kind: 'pane', tabs: [tab], active: tab }
          const before = command === 'split-left' || command === 'split-up'
          const branch = this.createSplit(
            command === 'split-left' || command === 'split-right' ? 'horizontal' : 'vertical',
            before ? next : pane,
            before ? pane : next,
          )
          this.layout = this.replacePane(present(this.layout), pane, branch)
        }
        this.layout = this.prune(this.layout)
        this.publishEntry(tab)
        this.renderLayout()
        await this.focusEntry(tab)
      } else {
        const index = pane.tabs.indexOf(tab)
        const closing =
          command === 'close'
            ? [tab]
            : pane.tabs.filter(
              (item, position) =>
                !item.pinned &&
                  (command === 'close-all' ||
                    (command === 'close-others' && item !== tab) ||
                    (command === 'close-left' && position < index) ||
                    (command === 'close-right' && position > index)),
            )
        if (closing.length > 1) {
          for (const item of closing) {
            await present(item.connection).prepareTabSwitch()
          }
        }
        if (epoch !== this.epoch) {
          return
        }
        for (const item of closing) {
          await this.closeEntry(item)
        }
        if (epoch !== this.epoch) {
          return
        }
        let next = this.visible(origin) ? origin : this.panes()[0]?.active
        if (!next) {
          const empty = await this.mount(origin.target, undefined, undefined, epoch, true)
          if (!empty) {
            return
          }
          this.layout = { kind: 'pane', tabs: [empty], active: empty }
          next = empty
        }
        this.publishEntry(next)
        this.renderLayout()
        await this.focusEntry(next)
      }
      await this.saveLayout()
    })
  }
  private removeTab(pane: Pane, entry: Entry) {
    const index = pane.tabs.indexOf(entry)
    pane.tabs.splice(index, 1)
    if (pane.active === entry && pane.tabs.length) {
      pane.active = present(pane.tabs[Math.min(index, pane.tabs.length - 1)])
    }
  }
  private async focusEntry(entry: Entry, focus?: FrameMount['focus']) {
    if (!this.anchor?.isConnected || !this.visible(entry)) {
      return
    }
    entry.frame.focus({ preventScroll: true })
    try {
      if (focus) {
        await present(entry.connection).focus(focus)
      } else {
        await present(entry.connection).activateTab()
      }
    } catch (error) {
      if (message(error) !== 'reference_keyboard_focus_unavailable') {
        throw error
      }
    }
  }
  /** Apply history to the focused native document.
   * @param redo - select redo rather than undo.
   * @returns whether an editor accepted the action.
   */
  async handleNativeHistory(redo: boolean): Promise<boolean> {
    const entry = Array.from(this.entries).find(item => item.frame === document.activeElement)
    if (!entry?.ready || !this.visible(entry) || !this.anchor?.isConnected) {
      return false
    }
    return present(entry.connection).history(redo)
  }
  /** Close the focused Wiki tab after its save queue drains.
   * @returns whether a focused native tab was handled.
   */
  async handleNativeClose(): Promise<boolean> {
    const entry = Array.from(this.entries).find(item => item.frame === document.activeElement)
    if (!entry?.document || entry.target.mode !== 'wikiDesktop' || !this.visible(entry) || !this.anchor?.isConnected) {
      return false
    }
    await this.tabIntent(entry, { command: 'close', tabId: entry.id }).catch((error: unknown) => {
      this.publish({ error: message(error) }) },
    )
    return true
  }
  /** Open or select a document, retaining tabs in the current Wiki page.
   * @param target - selected kernel generation and page.
   * @param selected - optional native document location.
   * @param focus - optional block and expected revision for keyboard focus.
   * @param reuseCurrent - replace an unpinned current tab after saving.
   * @returns completion of editor readiness and selection.
   */
  open(
    target: PortableTarget,
    selected?: DocumentKey,
    focus?: FrameMount['focus'],
    reuseCurrent = false,
  ): Promise<void> {
    const pending = this.pendingReference
    this.pendingReference = null
    if (pending && same(pending.target, target) && !selected && !focus) {
      selected = pending.location
      focus = { blockId: pending.location.blockId, hash: pending.location.hash }
    }
    const epoch = ++this.epoch
    return this.serialize(async () => {
      if (epoch !== this.epoch) {
        return
      }
      const previous = this.entry
      const retained = target.mode === 'wikiDesktop' && previous && !previous.disposed && same(previous.target, target)
      const pane = retained ? this.paneOf(previous) : undefined
      if (
        !focus &&
        previous &&
        !previous.disposed &&
        same(previous.target, target) &&
        (!selected || (previous.document && sameDoc(selected, previous.document)))
      ) {
        this.publishEntry(previous)
        return
      }
      this.publish({ phase: 'opening', error: null })
      try {
        const cached =
          selected && retained
            ? previous.document && sameDoc(previous.document, selected)
              ? previous
              : Array.from(this.entries).find(
                entry => entry.document && !entry.disposed && sameDoc(entry.document, selected),
              )
            : undefined
        if (pane && previous?.document) {
          await present(previous.connection).prepareTabSwitch()
        } else {
          await this.closeCurrent()
        }
        if (epoch !== this.epoch) {
          return
        }
        if (!cached && this.entries.size >= 32) {
          throw new Error('editor_tab_limit')
        }
        let entry = cached ?? (await this.mount(target, selected, focus, epoch))
        if (epoch !== this.epoch) {
          return
        }
        if (!entry) {
          this.publish({
            phase: 'ready',
            target,
            documents: [],
            selected: null,
            dirty: false,
            error: null,
          })
          return
        }
        if (!retained && target.mode === 'wikiDesktop' && entry.savedTabs) {
          const initial = entry
          try {
            entry = await this.restoreLayout(initial, epoch, !!selected || !!focus)
          } catch (error) {
            if (this.entries.has(initial)) {
              await this.closeEntry(initial)
            }
            if (epoch !== this.epoch) {
              return
            }
            throw error
          }
          if (epoch !== this.epoch) {
            return
          }
        }
        const nextPane = this.paneOf(entry) ?? (previous?.document ? pane : undefined)
        if (nextPane) {
          if (!nextPane.tabs.includes(entry)) {
            if (reuseCurrent && previous?.document && !previous.pinned) {
              const index = nextPane.tabs.indexOf(previous)
              // A failed disposal keeps the previous tab usable and retires
              // only the hidden replacement.
              try {
                await this.closeEntry(previous, false)
              } catch (error) {
                await this.closeEntry(entry)
                throw error
              }
              nextPane.tabs.splice(index, 1, entry)
            } else {
              nextPane.tabs.push(entry)
            }
          }
          nextPane.active = entry
        } else {
          this.layout = { kind: 'pane', tabs: [entry], active: entry }
        }
        this.publishEntry(entry)
        this.renderLayout()
        if (focus || pane) {
          await this.focusEntry(entry, focus)
        }
        if (retained) {
          await this.saveLayout()
        }
      } catch (error) {
        this.publish({ phase: 'error', error: message(error) })
        throw error
      }
    })
  }
  private async mount(
    target: PortableTarget,
    selected: DocumentKey | undefined,
    focus: FrameMount['focus'],
    epoch: number,
    empty = false,
  ): Promise<Entry | undefined> {
    const view = await this.host.openEditor(target.storeId, target.sessionId, target.pageId, target.mode)
    let entry: Entry | null = null
    try {
      if (
        view.storeId !== target.storeId ||
        view.sessionId !== target.sessionId ||
        view.pageId !== target.pageId ||
        view.mode !== target.mode
      ) {
        throw new Error('editor_identity_mismatch')
      }
      const result = await this.host.documents(target, '')
      const documents: DocumentItem[] = [...result.items]
      // Documents beyond the first list page require the same page capability.
      let cursor = result.nextCursor
      const cursors = new Set<string>()
      while (selected && !documents.some(item => sameDoc(item, present(selected))) && cursor) {
        if (epoch !== this.epoch) {
          await this.host.closeEditor(view)
          return
        }
        if (cursors.has(cursor) || documents.length >= 4096) {
          throw new Error('reference_lookup_limit')
        }
        cursors.add(cursor)
        const next = await this.host.documents(target, cursor)
        documents.push(...next.items)
        cursor = next.nextCursor
      }
      if (epoch !== this.epoch) {
        await this.host.closeEditor(view)
        return
      }
      if (!documents.length && target.mode === 'docxSingle') {
        await this.host.closeEditor(view)
        return
      }
      const choice = empty ? undefined : (selected ?? documents[0]),
        doc = choice ? { boxId: choice.boxId, docId: choice.docId } : null
      if (doc && !documents.some(item => sameDoc(item, doc))) {
        throw new Error('document_not_in_view')
      }
      const base = new URL(target.baseUrl, location.href)
      const hosted = target.baseUrl === '/api/wikiman'
      const frameOrigin = hosted ? location.origin : base.origin
      if (
        (!hosted && (base.protocol !== 'http:' || !['127.0.0.1', '[::1]'].includes(base.hostname))) ||
        base.username ||
        base.password ||
        base.pathname !== (hosted ? '/api/wikiman' : '/') ||
        base.search ||
        base.hash
      ) {
        throw new Error('invalid_editor_origin')
      }
      const channelId = randomUUID(),
        frame = document.createElement('iframe')
      frame.title = this.host.label('editor')
      frame.referrerPolicy = 'no-referrer'
      frame.style.cssText = 'position:absolute;height:100%;width:100%;border:0;visibility:hidden'
      frame.src =
        frameOrigin + '/stage/build/portable/#' + new URLSearchParams({ parentOrigin: location.origin, channelId }).toString()
      entry = {
        id: brandString<WikiTabId>(randomUUID()),
        target,
        view,
        frame,
        connection: null,
        reports: Promise.resolve(),
        disposed: false,
        ready: false,
        document: doc,
        documents,
        title: documents.find(item => doc && sameDoc(item, doc))?.title ?? doc?.docId ?? '',
        dirty: false,
        pinned: false,
      }
      const current = entry
      this.entries.add(current)
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => {
          window.removeEventListener('message', receive)
          reject(new Error('editor_boot_timeout'))
        }, 15000)
        const receive = (event: MessageEvent<unknown>) => {
          if (
            event.source !== frame.contentWindow ||
            event.origin !== frameOrigin ||
            !record(event.data) || event.data.type !== 'wiki:editor:booted' ||
            event.data.channelId !== channelId
          ) {
            return
          }
          clearTimeout(timer)
          window.removeEventListener('message', receive)
          try {
            current.connection = this.connect({
              frameWindow: present(frame.contentWindow),
              frameOrigin,
              channelId,
              payload: {
                baseUrl: hosted ? frameOrigin + '/api/wikiman' : base.origin,
                streamBaseUrl: (globalThis as { __DSH_TRANSPORT__?: { streamBaseUrl?: string } }).__DSH_TRANSPORT__?.streamBaseUrl,
                view,
                document: doc,
                locale: this.host.locale(),
                hostPluginLayout: false,
                ...(focus ? { focus } : {}),
              },
              navigate: (blockId, reuseCurrent) => this.navigate(current, blockId, reuseCurrent),
              tabIntent: intent => this.tabIntent(current, intent),
              savedTabs: (value) => {
                const parsed = parseSavedTabs(value)
                if (parsed) {
                  current.savedTabs = parsed
                }
              },
              tabTitle: (title) => {
                if (this.entries.has(current) && !current.disposed) {
                  current.title = title
                  this.renderLayout()
                }
              },
              treeDigest: (digest) => {
                if (this.entries.has(current) && !current.disposed) {
                  this.onTreeDigest?.(current.target, digest)
                }
              },
              pluginSystemShortcuts: (chords) => {
                if (this.entries.has(current) && !current.disposed) {
                  current.systemShortcuts = chords
                  if (this.entry === current) {
                    this.refreshShortcuts()
                  }
                }
              },
              pluginHostActions: (actions) => {
                if (this.entries.has(current) && !current.disposed) {
                  current.pluginActions = actions
                  if (this.entry === current) {
                    this.publishPluginActions()
                  }
                }
              },
              tabFocus: () => {
                if (
                  this.entry !== current &&
                  this.visible(current) &&
                  this.anchor?.isConnected &&
                  document.activeElement === frame
                ) {
                  this.publishEntry(current)
                  this.renderLayout()
                  void this.serialize(() => this.saveLayout())
                }
              },
              dirty: (dirty) => {
                if (!this.entries.has(current) || current.disposed) {
                  return
                }
                current.reports = current.reports
                  .then(() => {
                    // A normal dispose still drains reports already accepted
                    // by this lease before native close. Only a stopped and
                    // discarded/replaced owner may abandon queued reports.
                    if (this.entries.has(current)) {
                      return this.host.reportEditorDirty(view, dirty)
                    }
                  })
                  .catch(() => {
                    if (this.entries.has(current)) {
                      this.publish({ error: 'editor_dirty_report_failed' })
                    }
                  })
                current.dirty = dirty
                this.publish({ dirty: this.isDirty() })
                this.renderLayout()
              },
            })
            void current.connection.ready.then(() => {
              current.ready = true
              resolve()
            }, reject)
          } catch (error) {
            reject(error instanceof Error ? error : new Error(message(error)))
          }
        }
        window.addEventListener('message', receive)
        this.holder.appendChild(frame)
      })
      if (epoch !== this.epoch) {
        await this.closeEntry(current)
        return
      }
      await this.host.reportEditorDirty(view, false)
      if (epoch !== this.epoch) {
        await this.closeEntry(current)
        return
      }
      return current
    } catch (error) {
      if (entry?.ready) {
        try {
          await this.closeEntry(entry)
        } catch {
          throw error
        }
      } else {
        entry?.connection?.close()
        entry?.frame.remove()
        if (entry) {
          this.entries.delete(entry)
        }
        await this.host.closeEditor(view)
      }
      throw error
    }
  }
  private async navigate(entry: Entry, blockId: string, reuseCurrent = false): Promise<void> {
    const epoch = this.epoch
    const ownsSelection = () =>
      this.epoch === epoch && !!this.anchor?.isConnected && (!this.router || this.router.current(entry.target))
    const current = () => this.visible(entry) && entry.ready && ownsSelection()
    if (!current()) {
      throw new Error('stale_reference_navigation')
    }
    const link = await this.host.resolve(entry.target, blockId)
    if (!link) {
      throw new Error('reference_not_found')
    }
    const target: ReferenceLocation = {
      ...link.document,
      blockId: link.blockId,
      mode: link.mode,
    }
    if (!current()) {
      throw new Error('stale_reference_navigation')
    }
    if (target.pageId !== entry.target.pageId) {
      if (!this.router) {
        throw new Error('reference_navigation_unavailable')
      }
      await this.serialize(async () => {
        if (!current()) {
          throw new Error('stale_reference_navigation')
        }
        await this.closeCurrent()
        if (!ownsSelection()) {
          throw new Error('stale_reference_navigation')
        }
        this.pendingReference = {
          target: { ...entry.target, pageId: target.pageId, mode: target.mode },
          location: target,
        }
        this.publish({ phase: 'opening', dirty: false, error: null })
        try {
          present(this.router).select(target, entry.target)
        } catch (error) {
          this.pendingReference = null
          this.publish({ phase: 'error', error: message(error) })
          throw error
        }
      })
      return
    }
    this.entry = entry
    await this.open(
      { ...entry.target, mode: target.mode },
      target,
      target.blockId === target.docId ? undefined : { blockId: target.blockId, hash: target.hash },
      reuseCurrent,
    )
  }
  private async closeEntry(entry: Entry, removeFromLayout = true): Promise<void> {
    if (!entry.disposed) {
      if (!entry.connection) {
        throw new Error('editor_not_ready')
      }
      await entry.connection.dispose()
      entry.disposed = true
    }
    await entry.reports
    await this.host.closeEditor(entry.view)
    const pane = this.paneOf(entry)
    if (pane && removeFromLayout) {
      this.removeTab(pane, entry)
      this.layout = this.prune(this.layout)
    }
    entry.frame.remove()
    this.entries.delete(entry)
    if (this.entry === entry) {
      this.entry = this.panes()[0]?.active ?? null
    }
    this.refreshShortcuts()
    this.publishPluginActions()
    if (removeFromLayout) {
      this.renderLayout()
    }
  }
  private async closeCurrent(): Promise<void> {
    const entries = Array.from(this.entries)
    if (entries.length > 1) {
      for (const entry of entries) {
        if (!entry.disposed) {
          await present(entry.connection).prepareTabSwitch()
        }
      }
    }
    for (const entry of entries) {
      await this.closeEntry(entry)
    }
  }
  /** Drain and close all mounted and parked views before source maintenance.
   * @param control - source operation to invoke after all view leases close.
   * @returns the source operation result.
   */
  release<T>(control: () => Promise<T>): Promise<T> {
    this.pendingReference = null
    ++this.epoch
    return this.serialize(async () => {
      try {
        await this.closeCurrent()
        const result = await control()
        this.publish({
          phase: 'idle',
          target: null,
          selected: null,
          documents: [],
          dirty: false,
          error: null,
        })
        return result
      } catch (error) {
        this.publish({ phase: 'error', error: message(error) })
        throw error
      }
    })
  }
  /** Only an explicit user-confirmed discard may enter this path. Native stop
   * must succeed first; a failed/uncertain stop cannot authorize DOM loss.
   * @param stop - stops the native generation after user-confirmed discard.
   * @returns the native stop result.
   */
  discard<T>(stop: () => Promise<T>): Promise<T> {
    this.pendingReference = null
    ++this.epoch
    return this.serialize(async () => {
      const result = await stop()
      for (const entry of this.entries) {
        entry.connection?.close()
      }
      this.entries.clear()
      this.holder.replaceChildren()
      this.entry = null
      this.refreshShortcuts()
      this.publishPluginActions()
      this.layout = null
      this.publish({
        phase: 'idle',
        target: null,
        selected: null,
        documents: [],
        dirty: false,
        error: null,
      })
      return result
    })
  }
  /** Drain all editors and release browser positioning resources. */
  async dispose(): Promise<void> {
    await this.release(() => Promise.resolve())
    this.park()
    this.resizeObserver?.disconnect()
    window.removeEventListener('resize', this.position)
    window.removeEventListener('scroll', this.position, true)
    this.holder.remove()
    this.listeners.clear()
  }
}
