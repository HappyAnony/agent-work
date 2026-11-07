/** Wire values for the local WikiMan workbench. */

/** Stable WikiMan page identifier, kept as text to preserve its precision. */
export interface WikiPage {
  readonly pageId: string
  readonly parentId: string | null
  readonly name: string
  readonly type: 'wiki' | 'docx' | 'table'
  readonly rank: string
  readonly hash: string
  readonly deletedAt: string | null
}

/** Native location of a document within a WikiMan page. */
export interface WikiDocumentKey {
  readonly pageId: string
  readonly boxId: string
  readonly docId: string
}

/** A document row or a reference to a specific committed revision. */
export interface WikiDocument extends WikiDocumentKey {
  readonly storeId: string
  readonly title: string
  readonly hash: string
}

/** Continuation for a paged page or document listing. */
export interface WikiListing<T> {
  readonly items: readonly T[]
  readonly nextCursor: string
}

/** Kernel-issued capability scoped to one WikiMan page. */
export interface WikiEditorView {
  /** Editor API base, relative to the document in Web compositions and private loopback otherwise. */
  readonly baseUrl: string
  readonly view: {
    readonly storeId: string
    readonly pageId: string
    readonly sessionId: string
    readonly viewId: string
    readonly ticket: string
    readonly mode: 'wikiDesktop' | 'docxSingle'
  }
}

/** Editor view opened at one existing native document. */
export interface WikiEditor extends WikiEditorView {
  readonly document: WikiDocument
}

/** Native workspace view opens its file tree when no document is selected. */
export interface WikiPageEditor extends WikiEditorView {
  readonly document: WikiDocument | null
}

/** Host-validated native link with the target block and its committed document. */
export interface WikiLink {
  readonly mode: WikiEditorView['view']['mode']
  readonly document: WikiDocument
  readonly blockId: string
}

/** Idempotent request to create a page and its native entry document when applicable. */
export interface WikiCreatePage {
  readonly operationId: string
  readonly createdAt: string
  readonly name: string
  readonly type: 'wiki' | 'docx' | 'table'
  readonly parentId: string | null
}

/** Compare-and-set request from the page tree or recycle bin. */
export type WikiPageChange =
  | { readonly kind: 'rename'; readonly pageId: string; readonly hash: string; readonly name: string }
  | { readonly kind: 'move'; readonly pageId: string; readonly hash: string; readonly parentId: string | null; readonly rank: string }
  | { readonly kind: 'trash' | 'restore'; readonly pageId: string; readonly hash: string }

/** Outcome of synchronizing one store with its shared portable replica. */
export type WikiSyncResult =
  | { readonly kind: 'published' | 'received' | 'unchanged'; readonly digest: string; readonly warnings: number }
  | { readonly kind: 'conflict'; readonly localDigest: string; readonly remoteDigest: string }

/** Paths and user-supplied title for a new portable copy of a legacy knowledge base. */
export interface WikiLegacyImport {
  readonly source: string
  readonly destination: string
  readonly name: string
}

/** Verified import contents, retained identities and validation warnings. */
export interface WikiImportReport {
  readonly digest: string
  readonly storeId: string
  readonly destination: string | null
  readonly pages: number
  readonly documents: number
  readonly deletedLegacyPages: number
  readonly warnings: number
}

/** Confirmation of the exact portable bytes shown in an import preview. */
export interface WikiImportConfirmation {
  readonly storeId: string
  readonly digest: string
}

/** Authenticated browser entry for IronMan's unmodified SiYuan application. */
export interface WikiSiyuanView {
  readonly url: string
  readonly lifetimeUrl: string
  readonly origin: string
  readonly viewId: string
  readonly changed: boolean
}

/** The two supported native editor and storage formats. */
export type WikiRuntime = 'portable' | 'siyuan'
