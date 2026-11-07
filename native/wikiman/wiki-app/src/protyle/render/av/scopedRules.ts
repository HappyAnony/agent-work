import {Menu} from "../../../plugin/Menu";
import {escapeHtml} from "../../../util/escape";
import {genCellValue} from "./cell";
import {getDefaultOperatorByType, getFiltersHTML, setFilter} from "./filter";
import {getSortsHTML} from "./sort";
import {getFieldsByData} from "./view";
import {showMessage} from "../../../dialog/message";

export function openScopedRules(protyle: IProtyle, block: HTMLElement, anchor: HTMLElement, displayed: IAV,
                                kind: "filter" | "sort", commit: (operation: IOperation & {id:string}) => void) {
    if (protyle.disabled || !protyle.portable.canRestoreHistory) { return; }
    const data = structuredClone(displayed);
    const fields = getFieldsByData(data).filter(field => field.type !== "lineNumber" && /^\d{14}-[a-z0-9]{7}$/.test(field.id));
    data.view.filters ??= [];
    data.view.sorts ??= [];
    const rules = kind === "filter" ? data.view.filters : data.view.sorts;
    const position = () => {
        const rect = anchor.getBoundingClientRect();
        return {x:rect.left,y:rect.bottom,h:rect.height};
    };
    const persist = (next: IAVFilter[] | IAVSort[]) => commit({
        id:block.dataset.nodeId, avID:data.id, blockID:block.dataset.nodeId,
        action:kind === "filter" ? "setAttrViewFilters" : "setAttrViewSorts",
        data:next,
    });
    const editFilter = (filter: IAVFilter, empty: boolean) => {
        if (!anchor.isConnected || !protyle.portable.canRestoreHistory) { return; }
        protyle.portable.setAttributeCellDraft(true);
        const close = () => protyle.portable.setAttributeCellDraft(false);
        void setFilter({filter, empty, protyle, data, target:anchor, blockElement:block,
            scoped:{commit:persist,close}}).catch(error => {
            if (window.siyuan.menus.menu.element.getAttribute("data-name") === "set-filter-" + filter.column) {
                window.siyuan.menus.menu.remove();
            }
            close();
            showMessage(error instanceof Error ? error.message : window.siyuan.languages.operationFailed);
        });
    };
    const menu = new Menu("scoped-av-" + kind);
    if (menu.isOpen) { return; }
    const panel = document.createElement("div");
    panel.innerHTML = kind === "filter" ? getFiltersHTML(data) : getSortsHTML(fields, data.view.sorts);
    panel.querySelector('[data-type="go-config"]')?.remove();
    const container = panel.firstElementChild;
    container.querySelectorAll("select").forEach(useScopedRuleSelect);
    const indexOf = (row: Element) => rules.findIndex(rule => rule.column === row?.getAttribute("data-id") &&
        (kind === "sort" || (rule as IAVFilter).value.type === row?.getAttribute("data-filter-type")));
    const choose = () => {
        const choices = new Menu("scoped-av-add-" + kind);
        for (const field of fields) {
            if (rules.some(rule => rule.column === field.id)) { continue; }
            const operator = getDefaultOperatorByType(field.type);
            if (kind === "filter" && !operator) { continue; }
            choices.addItem({label:escapeHtml(field.name),click:()=>{
                queueMicrotask(() => {
                    if (kind === "sort") { persist([...data.view.sorts,{column:field.id,order:"ASC"}]); return; }
                    const filter: IAVFilter = {column:field.id,operator,
                        value:genCellValue(field.type, field.type === "checkbox" ? {checked:undefined} : "")};
                    data.view.filters.push(filter);
                    editFilter(filter,true);
                });
            }});
        }
        choices.open(position());
    };
    const submit = (next: IAVFilter[] | IAVSort[]) => {
        menu.close();
        persist(next);
    };
    container.addEventListener("click", event => {
        const target = event.target as Element;
        const type = target.closest("[data-type]")?.getAttribute("data-type");
        const index = indexOf(target.closest("[data-id]"));
        if (type === "addSort" || type === "addFilter") { choose(); }
        if (type === "removeSorts" || type === "removeFilters") { submit([]); }
        if (index >= 0 && (type === "removeSort" || type === "removeFilter")) {
            submit(kind === "sort" ? data.view.sorts.filter((_,i)=>i!==index) : data.view.filters.filter((_,i)=>i!==index));
        }
        if (index >= 0 && type === "setFilter") { editFilter(data.view.filters[index],false); }
    });
    container.addEventListener("change", event => {
        const select = event.target as HTMLSelectElement;
        const row = select.closest("[data-id]");
        const index = indexOf(row);
        if (kind !== "sort" || index < 0) { return; }
        const sort = data.view.sorts[index];
        if (row.querySelector("select") === select) { sort.column = select.value; }
        else { sort.order = select.value as IAVSort["order"]; }
        submit(data.view.sorts);
    });
    let from = -1;
    const clearDrop = () => container.querySelectorAll(".dragover__top,.dragover__bottom").forEach(row =>
        row.classList.remove("dragover__top","dragover__bottom"));
    container.addEventListener("dragstart", (event:DragEvent) => {
        from = indexOf((event.target as Element).closest("[data-id]"));
        if (from >= 0) { event.dataTransfer.setData("text/plain",""); event.dataTransfer.effectAllowed="move"; }
    });
    container.addEventListener("dragover", (event:DragEvent) => {
        const row = (event.target as Element).closest("[data-id]");
        clearDrop();
        if (from < 0 || indexOf(row) < 0 || indexOf(row) === from) { return; }
        event.preventDefault();
        const rect = row.getBoundingClientRect();
        row.classList.add(event.clientY < rect.top + rect.height / 2 ? "dragover__top" : "dragover__bottom");
    });
    container.addEventListener("drop", (event:DragEvent) => {
        const row = (event.target as Element).closest("[data-id]");
        const to = indexOf(row);
        if (from < 0 || to < 0 || to === from) { return; }
        event.preventDefault();
        const bottom = row.classList.contains("dragover__bottom");
        const [moved] = rules.splice(from,1);
        rules.splice(to + Number(bottom) - Number(from < to),0,moved as IAVSort & IAVFilter);
        submit(rules);
    });
    container.addEventListener("dragend", () => { from = -1; clearDrop(); });
    window.siyuan.menus.menu.append(container as HTMLElement);
    menu.open(position());
}

// The original select's data-URI background is outside the scoped image policy.
export function useScopedRuleSelect(select: HTMLSelectElement) {
    select.style.backgroundImage = "none";
    select.style.appearance = "auto";
}
