import {fetchSyncPost} from "../../util/fetch";
import {pathPosix} from "../../util/pathName";
import {appendDocExt, hasDocExt} from "../../util/docExt";
import {SessionError} from "../../portable/session";
import {captureScopedHintRange} from "./scopedRange";

async function createTarget(protyle: IProtyle, child: boolean) {
    const {boxId, docId} = protyle.portable.document;
    const untitled = window.siyuan.languages.untitled;
    if (child) { return {notebook: boxId, parentId: docId, title: untitled}; }
    const response = await fetchSyncPost("/api/filetree/getDocCreateSavePath", {notebook: boxId});
    if (response.code !== 0) { throw new SessionError("invalid_ui_document"); }
    const {box, path} = response.data as {box: string; path: string};
    const paths = pathPosix();
    const title = path.endsWith("/") ? untitled : paths.basename(path) || untitled;
    if (!path.includes("/") && box === boxId) { return {notebook: boxId, parentId: docId, title}; }
    if (path.startsWith("/") || protyle.path === "/" || !path.includes("/")) {
        return {notebook: box, title, hPath: paths.join("/", path, path.endsWith("/") || !path ? untitled : "")};
    }
    const current = hasDocExt(protyle.path) ? protyle.path : appendDocExt(protyle.path);
    const parent = await fetchSyncPost("/api/filetree/getHPathByPath", {notebook: box, path: box === boxId ? current : path});
    if (parent.code !== 0) { throw new SessionError("invalid_ui_document"); }
    return {notebook: box, title, hPath: paths.join(parent.data, path, path.endsWith("/") ? untitled : ""),
        parentId: box === boxId ? docId : undefined};
}

export async function createScopedDocReference(protyle: IProtyle, block: HTMLElement, range: Range, child: boolean) {
    const binding = protyle.portable;
    const capture = captureScopedHintRange(block, range);
    if (!capture) { return; }
    try {
        const target = await createTarget(protyle, child);
        binding.flushPendingInput();
        const prepared = capture(protyle);
        if (!binding.canRestoreHistory || !prepared) { return; }
        const {clone, selection} = prepared;
        const id = Lute.NewNodeID();
        const reference = document.createElement("span");
        reference.dataset.type = "block-ref";
        reference.dataset.subtype = "d";
        reference.dataset.id = id;
        reference.textContent = target.title;
        selection.insertNode(reference);
        await binding.createDocumentReference({...target, id, blockId: block.dataset.nodeId, blockDOM: clone.outerHTML});
    } catch {
        binding.reportUnavailableTool();
    }
}
