import {Dialog} from "../../dialog";
import {genAssetHTML} from "../../asset/renderAssets";
import {ExternalKind, externalURL} from "../../portable/external";
import {escapeAttr, escapeHtml} from "../../util/escape";
import {captureScopedHintRange} from "./scopedRange";

export function insertScopedExternal(protyle: IProtyle, block: HTMLElement, range: Range, kind: ExternalKind) {
    const capture = captureScopedHintRange(block, range);
    if (!capture) { return; }
    const binding = protyle.portable;
    const dialog = new Dialog({
        title: "URL",
        content: `<div class="b3-dialog__content"><input class="b3-text-field fn__block" type="url" aria-label="URL"><div role="status"></div></div>
<div class="b3-dialog__action"><button class="b3-button b3-button--text">${escapeHtml(window.siyuan.languages.confirm)}</button></div>`,
        width: "min(520px, 85vw)",
    });
    const field = dialog.element.querySelector<HTMLInputElement>("input");
    const confirm = dialog.element.querySelector<HTMLButtonElement>("button");
    field.focus();
    dialog.bindInput(field, () => confirm.click());
    confirm.addEventListener("click", async () => {
        const raw = field.value.trim();
        const url = externalURL(raw);
        if (!url) {
            dialog.element.querySelector("[role=status]").textContent = window.siyuan.languages.operationFailed;
            return;
        }
        const prepared = capture(protyle);
        if (!prepared || binding.session.status !== "active") { return; }
        const {clone, selection} = prepared;
        const id = block.dataset.nodeId;
        const escaped = escapeAttr(escapeHtml(raw));
        const label = escapeHtml(url.hostname);
        const html = kind === "iframe" ? protyle.lute.SpinBlockDOM(`<iframe src="${escaped}" sandbox=""></iframe>`) :
            genAssetHTML({image: ".png", audio: ".mp3", video: ".mp4"}[kind], escaped, label, label);
        const fragment = document.createElement("template");
        fragment.innerHTML = html;
        const element = fragment.content.firstElementChild as HTMLElement;
        if (!element) { return; }
        if (kind === "image") {
            selection.insertNode(element);
        } else if (!element.dataset.nodeId) {
            return;
        }
        dialog.destroy();
        await commitScopedHint(protyle, id, clone, kind === "image" ? undefined : element);
    });
}

export async function commitScopedHint(protyle: IProtyle, id: string, source: HTMLElement, newBlock?: HTMLElement) {
    const binding = protyle.portable;
    const operations: IOperation[] = [{action: "update", id, data: source.outerHTML}];
    if (newBlock) {
        operations.push({action: "insert", id: newBlock.dataset.nodeId, data: newBlock.outerHTML, previousID: id});
    }
    try {
        const history = await binding.menuTransaction(operations.map(op => ({...op, id: op.id})));
        if (history) {
            protyle.undo.add(operations, [], protyle);
            protyle.undo.undoStack[protyle.undo.undoStack.length - 1].nativeHistory = Promise.resolve(history);
        }
    } catch {
        binding.reportUnavailableTool();
    }
}
