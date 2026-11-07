import {scopedDiagramSelector, scopedRenderInteraction} from "../protyle/render/scopedSVG";
import {hideTooltip, showTooltip} from "../dialog/tooltip";
import {sanitizePortableMemo} from "./tools";

const memoSelector = '[data-type~="inline-memo"]';
const linkedInline = '[data-type~="a"], [data-type~="block-ref"], [data-type~="file-annotation-ref"]';
const borrowedContent = ".protyle-wysiwyg__embed, [data-portable-av-preview]";

/** A window-local menu with no legacy transport or ambient workspace lookup. */
export function installPortableMenu(options: {
    root: HTMLElement; content: HTMLElement;
    nativeBlockActions?: boolean;
    editable(): boolean; canUndo(): boolean; canRedo(): boolean;
    undo(): void; redo(): void; remove(block: HTMLElement): void;
    notice(message: string): void;
    contentMenu?(event: MouseEvent, block: HTMLElement): void;
    blockAttribute?(event: MouseEvent, block: HTMLElement, field: string): void;
    inlineMemo?(memo: HTMLElement): void;
    searchTag?(label: string): void;
    tagMenu?(event: MouseEvent, label: string): void;
    openAnnotation?(reference: string): void;
    codeOptions?(block: HTMLElement, trigger: HTMLElement): void;
}): () => void {
    const {root,content}=options, doc=root.ownerDocument;
    let menu:HTMLDivElement|null=null, live=true;
    const close=()=>{menu?.remove();menu=null;};
    const report=(message:string)=>{if(live){options.notice(message);}};
    const show=(event:MouseEvent,button:Element)=>{
        close();
        const id=button.getAttribute("data-node-id");
        const block=Array.from(content.querySelectorAll<HTMLElement>("[data-node-id]")).find(node=>node.dataset.nodeId===id);
        if(!block || !/^\d{14}-[a-z0-9]{7}$/.test(id??"")){report("此工具尚未适配共享知识仓，原始数据保持不变。");return;}
        const current=doc.createElement("div");menu=current;
        current.setAttribute("role","menu");current.setAttribute("aria-label","Wiki 块操作");
        current.style.cssText="position:fixed;z-index:10000;display:flex;flex-direction:column;gap:4px;padding:8px;background:#24272c;color:#eee;border:1px solid #626873;border-radius:8px;max-width:calc(100vw - 16px)";
        const valid=()=>live&&menu===current&&content.contains(block);
        const item=(label:string,enabled:boolean,action:()=>void)=>{
            const entry=doc.createElement("button");entry.type="button";entry.textContent=label;entry.setAttribute("role","menuitem");entry.disabled=!enabled;
            entry.onclick=()=>{if(!valid()){close();return;}close();action();};current.appendChild(entry);
        };
        item("复制块 ID",!!navigator.clipboard?.writeText,()=>{
            void navigator.clipboard.writeText(id!).catch(()=>report("复制失败，请检查剪贴板权限。"));
        });
        item("删除当前块",options.editable()&&block.parentElement===content,()=>{
            if(options.editable()&&content.contains(block)&&block.parentElement===content){options.remove(block);}
        });
        item("撤销",options.editable()&&options.canUndo(),()=>{if(options.editable()){options.undo();}});
        item("重做",options.editable()&&options.canRedo(),()=>{if(options.editable()){options.redo();}});
        const note=doc.createElement("small");note.textContent="仅删除当前顶层块。";current.appendChild(note);
        doc.body.appendChild(current);
        const rect=button.getBoundingClientRect(),x=event.type==="contextmenu"?event.clientX:rect.left,y=event.type==="contextmenu"?event.clientY:rect.bottom;
        current.style.left=Math.max(8,Math.min(x,doc.documentElement.clientWidth-current.offsetWidth-8))+"px";
        current.style.top=Math.max(8,Math.min(y,doc.documentElement.clientHeight-current.offsetHeight-8))+"px";
        current.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();
    };
    const isOriginalMultiSelection = (button: HTMLElement | null) => {
        if (!options.nativeBlockActions || !button?.dataset.nodeId || !options.editable()) { return false; }
        const blocks = Array.from(content.querySelectorAll<HTMLElement>(":scope > .protyle-wysiwyg--select"));
        return blocks.length > 1 && blocks.some(block => block.dataset.nodeId === button.dataset.nodeId) &&
            blocks.every(block => ["NodeParagraph", "NodeHeading"].includes(block.dataset.type));
    };
    const memoAt = (target: Element) => {
        const memo = target.closest<HTMLElement>(memoSelector);
        return memo && content.contains(memo) && !memo.closest(borrowedContent) ? memo : null;
    };
    const editMemo = (event: MouseEvent, target: Element) => {
        const memo = memoAt(target);
        if (!memo || !options.inlineMemo || !options.editable() ||
            event.type === "click" && target.closest(linkedInline)) { return false; }
        event.preventDefault();
        event.stopImmediatePropagation();
        close();
        hideTooltip();
        options.inlineMemo(memo);
        return true;
    };
    const tagAt = (target: Element) => target.closest<HTMLElement>('[data-type~="tag"]')
        ?.textContent.replace(/\u200b/g, "").trim();
    const context=(event:MouseEvent)=>{
        const target=event.target as Element;
        if(content.contains(target)){
            const tag = !target.closest(borrowedContent) && tagAt(target);
            if (tag && options.tagMenu && !event.shiftKey) {
                event.preventDefault();
                event.stopImmediatePropagation();
                close();
                options.tagMenu(event, tag);
                return;
            }
            if (editMemo(event, target)) { return; }
            if(target.closest("[data-portable-av-row]")){return;}
            const block = target.closest<HTMLElement>('[data-type="NodeParagraph"],[data-type="NodeHeading"]');
            const selection = doc.getSelection();
            const range = selection?.rangeCount ? selection.getRangeAt(0) : undefined;
            if (options.contentMenu && !event.shiftKey && block && range && block.contains(range.commonAncestorContainer) &&
                !target.closest('.protyle-wysiwyg__embed, [data-portable-av-preview], .img, [data-type~="block-ref"], [data-type~="a"], [data-type~="inline-math"]')) {
                event.preventDefault();
                event.stopImmediatePropagation();
                close();
                options.contentMenu(event, block);
                return;
            }
            // Do not cancel the browser's native copy/paste context menu.
            event.stopImmediatePropagation();close();return;
        }
        if(target.closest?.(".protyle-gutters")){
            const button = target.closest("button");
            if (isOriginalMultiSelection(button)) { return; }
            event.preventDefault();event.stopImmediatePropagation();show(event,button??target);
        }
    };
    const click=(event:MouseEvent)=>{
        const target=event.target as Element;
        if (event.type === "click" && content.contains(target) && !target.closest(borrowedContent)) {
            const annotation = target.closest<HTMLElement>('[data-type~="file-annotation-ref"]');
            if (annotation && options.openAnnotation && !doc.getSelection()?.toString()) {
                event.preventDefault();
                event.stopImmediatePropagation();
                options.openAnnotation(annotation.dataset.id ?? "");
                return;
            }
            const tag = tagAt(target);
            if (options.searchTag && tag && !event.altKey && !event.shiftKey && !doc.getSelection()?.toString()) {
                event.preventDefault();
                event.stopImmediatePropagation();
                options.searchTag(tag);
                return;
            }
            const trigger = target.closest<HTMLElement>(".protyle-action__menu");
            const code = trigger?.closest<HTMLElement>('[data-type="NodeCodeBlock"]');
            if (options.codeOptions && code && !code.dataset.subtype && options.editable() && code.querySelector(".hljs")) {
                event.preventDefault();
                event.stopImmediatePropagation();
                options.codeOptions(code, trigger);
                return;
            }
        }
        if(target.closest?.(".protyle-gutters")){
            const button = target.closest("button");
            const opensAttributes = event.shiftKey && button?.dataset.nodeId &&
                Array.from(content.querySelectorAll<HTMLElement>("[data-node-id]")).some(node =>
                    node.dataset.nodeId === button.dataset.nodeId && !node.closest(".protyle-wysiwyg__embed"));
            if (options.nativeBlockActions && options.editable() &&
                (button?.dataset.type === "fold" || opensAttributes || !event.altKey && isOriginalMultiSelection(button))) { return; }
            event.preventDefault();event.stopImmediatePropagation();show(event,button??target);
        }else if(target.closest?.(".protyle-attr")){
            const attrs = target.closest(".protyle-attr");
            const block = attrs.parentElement;
            event.preventDefault();
            event.stopImmediatePropagation();
            if (event.type !== "click" || !options.editable() || !content.contains(block) ||
                block.closest(borrowedContent) || !/^\d{14}-[a-z0-9]{7}$/.test(block.dataset.nodeId ?? "")) { return; }
            const field = ["bookmark", "name", "alias", "memo"].find(name =>
                attrs.contains(target.closest(".protyle-attr--" + name)));
            if (field) { options.blockAttribute?.(event, block, field); }
        }else if(event.type==="click" && editMemo(event,target)){
            return;
        }else if (options.inlineMemo && memoAt(target)?.matches(linkedInline)) {
            return;
        }else if(!options.nativeBlockActions&&event.type==="dblclick"&&target.closest?.("img:not(.emoji)")){
            event.preventDefault();event.stopImmediatePropagation();report("图片预览尚未适配共享知识仓；可使用当前图片或导出查看，原始数据保持不变。");
        }else if (options.nativeBlockActions && content.contains(target) && target.closest(".protyle-action__copy")) {
            const block = target.closest('[data-type="NodeCodeBlock"]');
            const code = block?.querySelector(".hljs");
            if (!code) { return; }
            event.preventDefault();
            event.stopImmediatePropagation();
            const reportFailure = () => report("复制失败，请检查剪贴板权限。");
            if (!navigator.clipboard?.writeText) { reportFailure(); return; }
            const text = code.textContent.replace(/\n$/, "").replace(/\u00a0/g, " ");
            void navigator.clipboard.writeText(text).then(() => report(window.siyuan.languages.copied), reportFailure);
        }else if(options.nativeBlockActions && content.contains(target) &&
            (target.closest(".protyle-action__language") || target.closest('[data-subtype="math"], [data-type~="inline-math"]') &&
                !target.closest(".protyle-action__menu, .protyle-action__reload, .protyle-wysiwyg__embed"))){
            return;
        }else if (target.closest(scopedRenderInteraction)) {
            return;
        }else if (options.nativeBlockActions && target.closest(".protyle-action__edit") &&
            target.closest(scopedDiagramSelector) &&
            !target.closest(".protyle-wysiwyg__embed")) {
            return;
        }else if (options.nativeBlockActions && target.closest("[data-portable-av-editor], [data-portable-av-action]")) {
            return;
        }else if(content.contains(target)&&target.closest?.('.protyle-action, .protyle-action__language, .protyle-action__menu, .protyle-action__reload, .protyle-action__edit, .protyle-action__copy, .render-node:not(.code-block), [data-portable-av-preview], .table__select, [data-type~="inline-math"], [data-subtype="math"], [data-type~="file-annotation-ref"], [data-type~="inline-memo"], [data-type~="tag"]')){
            // Code bodies remain editable. Legacy language/more/refresh/copy
            // controls, formula editors and dynamic blocks are not that body.
            event.preventDefault();event.stopImmediatePropagation();report("此特殊块工具尚未适配共享知识仓；原始数据保留，普通正文仍可编辑。");
        }
    };
    const memoHover = (event: MouseEvent) => {
        const memo = memoAt(event.target as Element);
        if (!memo || memo.contains(event.relatedTarget as Node)) { return; }
        if (event.type === "mouseout") {
            if (doc.getElementById("tooltip")?.classList.contains("tooltip--memo")) { hideTooltip(); }
        } else {
            showTooltip(sanitizePortableMemo(memo.dataset.inlineMemoContent ?? ""), memo, "memo");
        }
    };
    const drag=(event:DragEvent)=>{
        if ((event.target as Element).closest?.(".protyle-gutters") && (!options.nativeBlockActions || !options.editable() || !event.dataTransfer || !(event.target as HTMLElement).draggable)) {
            event.preventDefault();event.stopImmediatePropagation();report("块拖拽尚未适配共享知识仓。");
        }
    };
    const outside=(event:PointerEvent)=>{if(menu&&!menu.contains(event.target as Node)){close();}};
    const keys=(event:KeyboardEvent)=>{
        if(!menu){return;}
        if(event.key==="Escape"){event.preventDefault();event.stopImmediatePropagation();close();content.querySelector<HTMLElement>('[contenteditable="true"]')?.focus();return;}
        if(!["ArrowDown","ArrowUp","Home","End"].includes(event.key)){return;}
        event.preventDefault();event.stopImmediatePropagation();
        const entries=Array.from(menu.querySelectorAll<HTMLButtonElement>("button:not(:disabled)"));
        if(!entries.length){return;}
        const index=entries.indexOf(doc.activeElement as HTMLButtonElement);
        entries[event.key==="Home"?0:event.key==="End"?entries.length-1:(index+(event.key==="ArrowDown"?1:-1)+entries.length)%entries.length].focus();
    };
    root.addEventListener("contextmenu",context,true);root.addEventListener("click",click,true);root.addEventListener("dblclick",click,true);root.addEventListener("dragstart",drag,true);
    if (options.inlineMemo) {
        content.addEventListener("mouseover", memoHover);
        content.addEventListener("mouseout", memoHover);
    }
    doc.addEventListener("pointerdown",outside,true);doc.addEventListener("keydown",keys,true);
    return()=>{live=false;close();root.removeEventListener("contextmenu",context,true);root.removeEventListener("click",click,true);root.removeEventListener("dblclick",click,true);root.removeEventListener("dragstart",drag,true);doc.removeEventListener("pointerdown",outside,true);doc.removeEventListener("keydown",keys,true);
        content.removeEventListener("mouseover", memoHover);
        content.removeEventListener("mouseout", memoHover);
        if (options.inlineMemo && doc.getElementById("tooltip")?.classList.contains("tooltip--memo")) { hideTooltip(); }
    };
}
