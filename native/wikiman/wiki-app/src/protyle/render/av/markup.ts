import {genCellValue, genCellValueByElement, popTextCell} from "./cell";
import {addView, getFieldsByData} from "./view";
import {openScopedSelectCell} from "./select";
import {openScopedDateCell} from "./date";
import {openScopedRelationCell} from "./relation";
import type {ProtyleOperation} from "../../../portable/session";
import {showMessage} from "../../../dialog/message";
import {addCol} from "./col";
import {insertRows} from "./row";
import {avContextmenu} from "./action";
import {openScopedRules} from "./scopedRules";
import {updateSearch} from "./render";

const passiveTags = new Set("div span input button svg use img a p strong b em i u s del mark sub sup br pre code ul ol li table thead tbody tfoot tr th td caption section".split(" "));
const scalarTypes = new Set(["text", "email", "phone", "url", "number"]);
const selectTypes = new Set(["select","mSelect"]);
const nativeID = /^\d{14}-[a-z0-9]{7}$/;
const displayedViews = new WeakMap<HTMLElement,IAV>();

// Original AV templates contain source-derived field values and cover markup.
// A scoped editor stages their DOM before attaching it to the active document.
export async function writeAVMarkup(protyle: IProtyle, target: Element, html: string, replace = false): Promise<boolean> {
    if (protyle.portable) {
        const template = document.createElement("template");
        template.innerHTML = html;
        for (const node of template.content.querySelectorAll("*")) {
            if (!passiveTags.has(node.localName)) { node.remove(); continue; }
            for (const attr of ["autofocus", "accesskey", "tabindex", "popover", "draggable", "download", "target"]) {
                node.removeAttribute(attr);
            }
            if (node.localName !== "use") {
                node.removeAttribute("href");
                node.removeAttribute("xlink:href");
            }
            if (node.localName === "input" || node.localName === "button") {
                node.setAttribute("disabled", "");
            }
        }
        for (const node of template.content.querySelectorAll<HTMLElement>("[style]")) {
            for (const property of Array.from(node.style)) {
                const value = node.style.getPropertyValue(property);
                if (["position", "z-index", "visibility", "pointer-events"].includes(property) || /url\s*\(|image-set\s*\(|[\\@]/i.test(value)) {
                    node.style.removeProperty(property);
                }
            }
        }
        template.content.querySelectorAll("use").forEach(node => {
            const href = node.getAttribute("href") || node.getAttribute("xlink:href");
            if (!/^#icon[A-Za-z0-9]+$/.test(href ?? "")) { node.remove(); }
        });
        template.content.querySelectorAll<HTMLImageElement>("img[src]").forEach(node => {
            const source = node.getAttribute("src")?.split("?")[0];
            if (/^\/?assets\//.test(source ?? "")) { node.setAttribute("src", source); }
            else { node.removeAttribute("src"); }
        });
        html = await protyle.portable.prepareReadOnlyUIFragment(template.innerHTML, protyle.portable.document);
        if (!protyle.wysiwyg.element.contains(target)) { return false; }
    }
    if (replace) { target.outerHTML = html; }
    else { target.innerHTML = html; }
    return true;
}

export function completeAVPreview(protyle: IProtyle, block: HTMLElement, data: IAV, callback?: (data: IAV) => void): boolean {
    if (!protyle.portable) { return false; }
    block.setAttribute("data-render", "true");
    block.dataset.avType = data.viewType;
    block.setAttribute("data-portable-av-preview", "");
    block.contentEditable = "false";
    displayedViews.set(block,data);
    bindSearch(protyle,block);
    if (/^[0-9a-f]{64}$/.test(data.sourceHash ?? "")) {
        bindCells(protyle, block, data);
        bindViewActions(protyle, block, data);
    }
    callback?.(data);
    return true;
}

function bindSearch(protyle: IProtyle, block: HTMLElement) {
    const input=block.querySelector<HTMLInputElement>('input[data-type="av-search"]');
    const icon=block.querySelector<HTMLButtonElement>('button[data-type="av-search-icon"]');
    const views=input?.closest(".av__views");
    if (!input || !icon || !views || input.dataset.portableAvAction!==undefined) { return; }
    for (const control of [input,icon]) {
        control.dataset.portableAvAction="";
        control.disabled=false;
    }
    icon.addEventListener("click",event=>{
        event.stopPropagation();
        input.style.width="128px";
        input.style.removeProperty("padding-left");
        input.style.removeProperty("padding-right");
        views.classList.add("av__views--show");
        input.focus();
    });
    input.addEventListener("click",event=>event.stopPropagation());
    input.addEventListener("keydown",event=>{
        event.stopPropagation();
        if (event.key==="Escape"&&!event.isComposing) {
            event.preventDefault();
            input.value="";
            updateSearch(block,protyle);
            protyle.wysiwyg.element.focus();
        }
    });
    input.addEventListener("input",event=>{
        event.stopPropagation();
        if (!(event as InputEvent).isComposing) { updateSearch(block,protyle); }
    });
    for (const type of ["compositionstart","compositionupdate","compositionend"]) {
        input.addEventListener(type,event=>{
            event.stopPropagation();
            if (type==="compositionend") { updateSearch(block,protyle); }
        });
    }
    input.addEventListener("blur",()=>{
        if (input.value) { return; }
        views.classList.remove("av__views--show");
        input.style.width="0";
        input.style.paddingLeft="0";
        input.style.paddingRight="0";
    });
}

function submitAVEdit(protyle: IProtyle, block: HTMLElement, data: IAV, operations: (IOperation & ProtyleOperation)[]) {
    if (protyle.disabled || !block.isConnected || !protyle.portable.canRestoreHistory) { return; }
    protyle.wysiwyg.element.focus({preventScroll:true});
    void protyle.portable.updateAttributeView(operations, {blockId:block.dataset.nodeId, hash:data.sourceHash}).then(history => {
        protyle.undo.add(operations, [], protyle);
        protyle.undo.undoStack[protyle.undo.undoStack.length - 1].nativeHistory = Promise.resolve(history);
    }).catch(error => {
        showMessage(error instanceof Error ? error.message : window.siyuan.languages.operationFailed);
    });
}

function bindViewActions(protyle: IProtyle, block: HTMLElement, data: IAV) {
    const {nodeId:blockID, avId:avID} = block.dataset;
    if (!nativeID.test(blockID ?? "") || !nativeID.test(avID ?? "")) { return; }
    const commit = (operations: (IOperation & ProtyleOperation)[]) => submitAVEdit(protyle, block, displayedViews.get(block)??data, operations);
    const enable = (control: HTMLElement, action: () => void) => {
        if (protyle.disabled || control.dataset.portableAvAction!==undefined) { return; }
        control.dataset.portableAvAction = "";
        control.removeAttribute("disabled");
        if (control.tagName !== "BUTTON") {
            control.tabIndex = 0;
            control.setAttribute("role","button");
            control.addEventListener("keydown", event => {
                if (!event.isComposing && (event.key === "Enter" || event.key === " ")) {
                    event.preventDefault();event.stopPropagation();action();
                }
            });
        }
        control.addEventListener("click", event => {
            event.preventDefault();event.stopPropagation();action();
        });
    };
    for (const tab of block.querySelectorAll<HTMLElement>(".av__views .layout-tab-bar > .item:not(.item--focus)")) {
        const id = tab.dataset.id;
        if (data.views.some(view => view.id === id) && nativeID.test(id ?? "")) {
            enable(tab, () => commit([{action:"setAttrViewBlockView", blockID, avID, id}]));
        }
    }
    for (const button of block.querySelectorAll<HTMLElement>('[data-type="av-add-more"], [data-type="av-add-top"], [data-type="av-add-bottom"]')) {
        enable(button, () => {
            const body = button.closest(".av__body");
            const group = button.closest(".av__group-title")?.nextElementSibling;
            const previousID = button.dataset.type === "av-add-bottom" ?
                body?.querySelector(".av__row--util")?.previousElementSibling?.getAttribute("data-id") ??
                button.previousElementSibling?.getAttribute("data-id") ?? "" : "";
            const groupID = (group ?? body ?? block.querySelector(".av__body"))?.getAttribute("data-group-id") ?? "";
            insertRows({blockElement:block, protyle, count:1, previousID, groupID, commit});
        });
    }
    for (const button of block.querySelectorAll<HTMLElement>('[data-type="av-header-add"]')) {
        enable(button, () => {
            const rect = button.getBoundingClientRect();
            addCol(protyle, block, undefined, commit).open({x:rect.left,y:rect.bottom,h:rect.height});
        });
    }
    for (const kind of ["filter","sort"] as const) {
        const control = block.querySelector<HTMLElement>(`[data-type="av-${kind}"]`);
        if (control) {
            enable(control, () => {
                const current = displayedViews.get(block) ?? data;
                openScopedRules(protyle,block,control,current,kind,operation => submitAVEdit(protyle,block,current,[operation]));
            });
        }
    }
    const addViewButton = block.querySelector<HTMLElement>('.av__views [data-type="av-add"]');
    if (addViewButton) { enable(addViewButton, () => addView(protyle, block, commit)); }
    if (!protyle.disabled) {
        for (const row of block.querySelectorAll<HTMLElement>(".av__row[data-id], .av__gallery-item[data-id]")) {
            if (!nativeID.test(row.dataset.id ?? "") || row.classList.contains("av__row--header") ||
                row.dataset.portableAvRow!==undefined) { continue; }
            row.dataset.portableAvRow = "";
            row.addEventListener("contextmenu", event => {
                event.preventDefault();event.stopPropagation();
                if (protyle.disabled || !protyle.portable.canRestoreHistory) { return; }
                avContextmenu(protyle,row,{x:event.clientX,y:event.clientY},commit);
            });
        }
    }
}

function bindCells(protyle: IProtyle, block: HTMLElement, data: IAV) {
    const binding = protyle.portable;
    const fields = getFieldsByData(data);
    for (const cell of block.querySelectorAll<HTMLElement>(".av__row[data-id] .av__cell[data-id], .av__gallery-item[data-id] .av__cell[data-id]")) {
        const rowId = cell.closest<HTMLElement>(".av__row, .av__gallery-item")?.dataset.id;
        const {id, colId = cell.dataset.fieldId} = cell.dataset;
        const type = cell.dataset.dtype as TAVCol;
        const field = fields.find(item => item.id === colId && item.type === type);
        if (![id, rowId, colId, block.dataset.nodeId, block.dataset.avId].every(value => nativeID.test(value ?? "")) ||
            !field || !scalarTypes.has(type) && !selectTypes.has(type) && type !== "checkbox" && type !== "date" &&
                !(type==="relation"&&nativeID.test(field.relation?.avID ?? ""))) { continue; }
        cell.dataset.portableAvEditor = "";
        cell.tabIndex = 0;
        const submit = (value?: IAVCellValue) => {
            binding.setAttributeCellDraft(false);
            if (!value || protyle.disabled || !cell.isConnected) { return; }
            const operation: IOperation & ProtyleOperation = {
                action:"updateAttrViewCell", id, avID:block.dataset.avId, keyID:colId, rowID:rowId,
                data:{...value,id} as unknown as ProtyleOperation["data"],
            };
            submitAVEdit(protyle, block, data, [operation]);
        };
        cell.addEventListener("click", event => {
            event.stopPropagation();
            cell.focus({preventScroll: true});
        });
        const edit = (event: Event) => {
            if (protyle.disabled || !binding.canRestoreHistory || document.querySelector(".av__mask, .av__panel")) { return; }
            event.preventDefault();
            event.stopPropagation();
            if(type==="checkbox"){
                submit({type,checkbox:{checked:!genCellValueByElement(type,cell).checkbox.checked}});
                return;
            }
            if(selectTypes.has(type)){
                trackPanel(openScopedSelectCell(cell,field,submit));
                return;
            }
            if(type==="date"){
                trackPanel(openScopedDateCell(protyle,block,cell,submit));
                return;
            }
            if(type==="relation"){
                trackPanel(openScopedRelationCell(protyle,data,cell,field,submit));
                return;
            }
            popTextCell(protyle, [cell], type, value => {
                submit(value===undefined?undefined:genCellValue(type,value));
            });
            trackPanel(document.querySelector(".av__mask"));
        };
        const trackPanel=(panel:Element|null)=>{
            if(!panel){return;}
            binding.setAttributeCellDraft(true);
            const observer=new MutationObserver(()=>{
                if(!panel.isConnected){observer.disconnect();binding.setAttributeCellDraft(false);}
            });
            observer.observe(document.body,{childList:true});
        };
        cell.addEventListener("dblclick", edit);
        cell.addEventListener("keydown", event => { if (event.key === "Enter" && !event.isComposing) { edit(event); } });
    }
}
