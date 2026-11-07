/**
 * note-syuan Single Notebook Mode — bundle entry.
 *
 * Loaded inside an iframe served at /stage/build/single/. Performs four-branch
 * boot validation against /api/system/getConf, then mounts a single Protyle
 * editor against the kernel's unique root document. On any failure, renders
 * an inline fallback message in the document body.
 *
 * See docs/feature/siyuan/Engineering/fe/05-boot-entry.md for the spec.
 *
 * MUST NOT import from:
 *   - ../layout/    ../layout/dock/    (no Files/Dock/topBar/Layouts)
 *   - ../menus/     ../dialog/         (except internal Protyle dialogs under ../protyle/dialog/)
 *   - ../window/    ../mobile/
 */
import "../assets/scss/base.scss";
