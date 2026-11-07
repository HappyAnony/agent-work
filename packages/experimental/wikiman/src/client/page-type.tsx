/** Names and existing shared glyphs for WikiMan page types. */
import { FileTypeIcon, IconBrowseOutlineRegular, IconWorkspaceTreeOutlineRegular } from '@deepseek-ai/dsh-client-ui-primitives'
import type { WikiPage } from '../types.ts'

/** Locale keys for the displayed page types; native documents retain the stored `docx` type. */
export const pageTypeLabels = { docx: 'docxPage', wiki: 'wikiPage', table: 'tablePage' } as const

/**
 * Render the decorative glyph identifying a page's editing function.
 * @param props - native page type and the rendered icon size in pixels.
 * @returns the shared document, workspace or spreadsheet glyph.
 */
export function PageTypeIcon({ type, size = 14 }: { readonly type: WikiPage['type']; readonly size?: number }) {
  return {
    docx: <IconBrowseOutlineRegular size={size} />,
    wiki: <IconWorkspaceTreeOutlineRegular size={size} />,
    table: <FileTypeIcon kind="excel" size={size} />,
  }[type]
}
