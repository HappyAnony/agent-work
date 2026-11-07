import {Constants} from "../constants";
import {Dialog} from "../dialog";
import {escapeAriaLabel, escapeHtml} from "../util/escape";
import {fetchSyncPost} from "../util/fetch";
import {addClearButton} from "../util/addClearButton";
import {scopedUIOpenDocument} from "../portable/providers";
import type {ProtyleBinding} from "../portable/protyle";
import {focusByRange} from "../protyle/util/selection";
import {assetFilterMenu, assetMoreMenu, renderAssetSearchLayout} from "./assets";
import {getDefaultType} from "./getDefault";
import {filterMenu, moreMenu, queryMenu} from "./menu";
import {genQueryHTML, renderBlockSearchRows, renderSearchLayout} from "./util";

type AssetHit = {id: string; name: string; path: string; ext: string; hSize: string; content: string};
type Hits = {blocks?: IBlock[]; assetContents?: AssetHit[]; pageCount?: number;
    matchedBlockCount?: number; matchedRootCount?: number; matchedAssetCount?: number; docMode?: boolean};
type SearchState = {
    panel: HTMLElement; input: HTMLInputElement; list: HTMLElement; preview: HTMLElement;
    count: HTMLElement; previous: HTMLElement; next: HTMLElement; assets: boolean;
    page: number; pageCount: number; results: Map<string, HTMLElement>;
};

const blockID = /^\d{14}-[a-z0-9]{7}$/;

// The original search service emits marked snippets. Keep only the two
// presentation tags it needs before passing results into the original rows.
function markedText(value: string) {
    const template = document.createElement("template");
    template.innerHTML = (value ?? "").replace(/<(?!\/?mark>|br>)/gi, "&lt;");
    return template.innerHTML;
}

function scopedBlock(block: IBlock): IBlock {
    return {...block, ial: {}, refCount: Number(block.refCount) || 0, content: markedText(block.content),
        name: markedText(block.name), alias: markedText(block.alias), memo: markedText(block.memo),
        tag: escapeHtml(block.tag), children: block.children?.filter(validBlock).map(scopedBlock)};
}

function validBlock(block: IBlock) {
    return block && blockID.test(block.id) && blockID.test(block.rootID) && blockID.test(block.box);
}

export function openScopedSearch(binding?: ProtyleBinding, keyword?: string) {
    const existing = document.querySelector<HTMLInputElement>(".b3-dialog--open [data-scoped-search] > :not(.fn__none) :is(#searchInput,#searchAssetInput)");
    if (existing) {
        if (keyword === undefined) { existing.select(); return; }
        window.siyuan.dialogs.find(item => item.element.contains(existing))?.destroy({focus:"false"});
    }
    const config: Config.IUILayoutTabSearchConfig = {
        k: "", r: "", hPath: "", idPath: [], types: getDefaultType(), replaceTypes: {}, page: 1,
        method: 0, sort: 0, group: 0, hasReplace: false, removed: true,
    };
    const assets: ISearchAssetOption = {
        k: "", keys: [], col: "", row: "", layout: 0, method: 0, sort: 0,
        types: Object.fromEntries(Constants.WIKI_ASSETS_SEARCH.map(ext => [ext, true])) as ISearchAssetOption["types"],
    };
    const languages = window.siyuan.languages;
    const selection = getSelection();
    const previousRange = selection.rangeCount ? selection.getRangeAt(0).cloneRange() : undefined;
    const previousFocus = document.activeElement as HTMLElement;
    let generation = 0, previewGeneration = 0, timer = 0;
    const dialog = new Dialog({
        title: languages.globalSearch, width: "80vw", height: "85vh", disableAnimation: true,
        content: '<div data-scoped-search style="height:100%"></div>',
        destroyCallback(options) {
            clearTimeout(timer);
            generation++;
            previewGeneration++;
            if (options?.focus === "false") { return; }
            if (previousRange?.startContainer.isConnected &&
                previousFocus?.classList.contains("protyle-wysiwyg") && binding?.session.status === "active") {
                focusByRange(previousRange);
            } else if (previousFocus?.isConnected) {
                previousFocus.focus({preventScroll: true});
            }
        },
    });
    const root = dialog.element.querySelector<HTMLElement>("[data-scoped-search]");
    renderSearchLayout(config, root, true);
    const blockPanel = root.firstElementChild as HTMLElement;
    const assetPanel = root.querySelector<HTMLElement>("#searchAssets");
    renderAssetSearchLayout(assetPanel, false, assets);
    const loading = root.querySelector(".fn__loading--top");

    const states = [blockPanel, assetPanel].map((panel, index): SearchState => {
        const prefix = index ? "searchAsset" : "search";
        return {
            panel, input: panel.querySelector("#" + prefix + "Input"), list: panel.querySelector("#" + prefix + "List"),
            preview: panel.querySelector("#" + prefix + "Preview"), count: panel.querySelector("#" + prefix + "Result"),
            previous: panel.querySelector(index ? '[data-type="assetPrevious"]' : '[data-type="previous"]'),
            next: panel.querySelector(index ? '[data-type="assetNext"]' : '[data-type="next"]'),
            assets: index === 1, page: 1, pageCount: 1, results: new Map(),
        };
    });
    let current = states[0];
    const visible = () => root.isConnected && dialog.element.classList.contains("b3-dialog--open");
    const selected = () => current.list.querySelector<HTMLElement>(".b3-list-item--focus[data-type='search-item']");
    blockPanel.querySelectorAll("#searchPathInput,#searchPath,#searchInclude,#searchOpen,#searchUnRef,#searchReplace,#criteria,.search__tip").forEach(node => node.classList.add("fn__none"));
    blockPanel.querySelectorAll(".search__header")[1].classList.add("fn__none");
    root.querySelectorAll(".search__drag,#searchHistoryBtn,#assetHistoryBtn").forEach(node => node.classList.add("fn__none"));
    for (const state of states) {
        state.preview.setAttribute("inert", "");
        state.preview.classList.add("b3-typography");
        state.preview.style.overflow = "auto";
        state.count.setAttribute("aria-live", "polite");
    }

    async function select(row: HTMLElement) {
        const state = current, seq = ++previewGeneration;
        selected()?.classList.remove("b3-list-item--focus");
        row.classList.add("b3-list-item--focus");
        row.scrollIntoView({block: "nearest"});
        state.preview.replaceChildren();
        const id = row.dataset.nodeId || row.dataset.id;
        if (state.assets) {
            const response = await fetchSyncPost("/api/search/getAssetContent", {id, query: state.input.value, method: assets.method});
            if (visible() && seq === previewGeneration && response.code === 0 && response.data) {
                state.preview.innerHTML = "<p>" + markedText(response.data.content) + "</p>";
            }
        } else if (binding) {
            const target = await fetchSyncPost("/api/block/getBlockInfo", {id});
            if (!visible() || seq !== previewGeneration || target.code !== 0 || !target.data?.rootID) { return; }
            const response = await fetchSyncPost("/api/block/getBlockDOM", {id});
            if (!visible() || seq !== previewGeneration || response.code !== 0 || typeof response.data?.dom !== "string") { return; }
            const html = await binding.prepareReadOnlyUIFragment(response.data.dom, {boxId: target.data.box, docId: target.data.rootID});
            if (visible() && seq === previewGeneration) { state.preview.innerHTML = html; }
        }
    }

    function showPreview(row: HTMLElement) {
        void select(row).catch(() => undefined); // Owner or source may have retired during the read.
    }

    function search(page = 1, delay = 0) {
        clearTimeout(timer);
        const state = current, seq = ++generation, options = state.assets ? assets : config;
        previewGeneration++;
        state.results.clear();
        state.list.replaceChildren();
        state.preview.replaceChildren();
        state.previous.setAttribute("disabled", "");
        state.next.setAttribute("disabled", "");
        loading.classList.remove("fn__none");
        const query = state.input.value;
        if (state.assets && !query.trim()) {
            state.page = state.pageCount = 1;
            loading.classList.add("fn__none");
            state.count.textContent = "";
            state.list.textContent = languages.emptyContent;
            return;
        }
        const recent = !state.assets && !query.trim();
        timer = window.setTimeout(async () => {
            const endpoint = recent ? "/api/block/getRecentUpdatedBlocks" : state.assets
                ? "/api/search/fullTextSearchAssetContent" : "/api/search/fullTextSearchBlock";
            const response = await fetchSyncPost(endpoint, recent ? {} : {
                query, method: options.method, orderBy: options.sort, page, pageSize: 32, types: options.types,
                ...(state.assets ? {} : {groupBy: config.group, paths: config.idPath}),
            });
            if (!visible() || seq !== generation) { return; }
            loading.classList.add("fn__none");
            if (response.code !== 0) {
                state.count.textContent = response.msg;
                return;
            }
            const result: Hits = response.data;
            state.page = page;
            state.pageCount = result.pageCount || 1;
            state.previous.toggleAttribute("disabled", page === 1);
            state.next.toggleAttribute("disabled", page >= state.pageCount);
            if (state.assets) {
                state.list.innerHTML = (result.assetContents ?? []).filter(row => blockID.test(row.id) || /^[0-9a-f]{64}$/.test(row.id)).map(row =>
                    `<div data-type="search-item" class="b3-list-item" data-id="${row.id}">
<span class="ft__on-surface">${escapeHtml(row.ext)}</span><span class="fn__space"></span>
<span class="b3-list-item__text">${markedText(row.content)}</span><span class="b3-list-item__meta">${escapeHtml(row.hSize)}</span>
<span class="b3-list-item__meta b3-list-item__meta--ellipsis ariaLabel" aria-label="${escapeAriaLabel(row.path)}">${markedText(row.name)}</span></div>`).join("");
            } else {
                state.list.innerHTML = renderBlockSearchRows((result.blocks ?? []).filter(validBlock).map(scopedBlock)).resultHTML;
            }
            const summary = state.assets ? languages.total + " " + (result.matchedAssetCount ?? 0) :
                (result.docMode ? languages.matchDoc : languages.findInDoc)
                    .replace("${x}", String(result.matchedRootCount ?? 0)).replace("${y}", String(result.matchedBlockCount ?? 0));
            state.count.textContent = recent ? "" : `${page}/${state.pageCount}  ${summary}`;
            state.list.querySelectorAll<HTMLElement>("[data-type='search-item']").forEach(row => {
                state.results.set(row.dataset.nodeId || row.dataset.id, row);
            });
            const first = state.results.values().next().value;
            if (first) { showPreview(first); }
            else { state.list.textContent = languages.emptyContent; }
        }, delay);
    }

    function switchPanel(state: SearchState) {
        current = state;
        states.forEach(item => item.panel.classList.toggle("fn__none", item !== state));
        state.input.select();
        search();
    }

    function openResult(row: HTMLElement) {
        if (current.assets) { return; }
        dialog.destroy({focus: "false"});
        if (binding && row.dataset.rootId === binding.document.docId) {
            try { binding.focusReference(row.dataset.nodeId); }
            catch (error) { if (error?.message !== "reference_keyboard_focus_unavailable") { binding.reportUnavailableTool(); } }
            return;
        }
        void scopedUIOpenDocument(row.dataset.nodeId).catch(() => undefined);
    }

    root.addEventListener("click", (event) => {
        const target = (event.target as Element).closest<HTMLElement>("[id],[data-type],.b3-list-item__toggle");
        if (!target || !current.panel.contains(target) || target.hasAttribute("disabled")) { return; }
        const row = target.closest<HTMLElement>("[data-type='search-item']");
        if (row) { showPreview(row); return; }
        const {id, dataset} = target;
        if (id === "searchAsset") { switchPanel(states[1]); }
        else if (id === "searchAssetClose") { switchPanel(states[0]); }
        else if (target === current.previous) { search(current.page - 1); }
        else if (target === current.next) { search(current.page + 1); }
        else if (id === "searchRefresh" || dataset.type === "assetRefresh") { search(current.page); }
        else if (id === "searchFilter") { filterMenu(config, () => search()); }
        else if (id === "assetFilter") { assetFilterMenu(assetPanel, {types: assets.types, refresh: () => search()}); }
        else if (id === "searchSyntaxCheck" || id === "assetSyntaxCheck") {
            const options = current.assets ? assets : config;
            queryMenu(options, () => { target.outerHTML = genQueryHTML(options.method, id); search(); }, true);
            const rect = target.getBoundingClientRect();
            window.siyuan.menus.menu.popup({x: rect.right, y: rect.bottom, isLeft: true});
        } else if (id === "searchMore") {
            void moreMenu(config, [], root, () => search(), () => undefined, undefined, true);
            const rect = target.getBoundingClientRect();
            window.siyuan.menus.menu.popup({x: rect.right, y: rect.bottom, isLeft: true});
        } else if (id === "assetMore") { assetMoreMenu(target, assetPanel, () => search(), assets); }
        else if (id === "searchExpand" || id === "searchCollapse" || target.classList.contains("b3-list-item__toggle")) {
            const arrows = target.classList.contains("b3-list-item__toggle") ? target.querySelectorAll(".b3-list-item__arrow") : current.list.querySelectorAll(".b3-list-item__arrow");
            arrows.forEach(arrow => {
                const expanded = id === "searchExpand" || id !== "searchCollapse" && !arrow.classList.contains("b3-list-item__arrow--open");
                arrow.classList.toggle("b3-list-item__arrow--open", expanded);
                arrow.closest(".b3-list-item").nextElementSibling?.classList.toggle("fn__none", !expanded);
            });
        }
    });
    root.addEventListener("dblclick", event => {
        const row = (event.target as Element).closest<HTMLElement>("[data-type='search-item']");
        if (row && current.list.contains(row)) { openResult(row); }
    });
    for (const state of states) {
        const refresh = () => search(1, Constants.TIMEOUT_INPUT);
        state.input.addEventListener("input", (event: InputEvent) => { if (!event.isComposing) { refresh(); } });
        state.input.addEventListener("compositionend", refresh);
        addClearButton({inputElement: state.input, right: 8, height: 28, clearCB: refresh});
        state.input.addEventListener("keydown", event => {
            if (event.isComposing) { return; }
            const row = selected();
            if (event.key === "Escape") { dialog.destroy(); }
            else if (event.key === "Enter" && row && !current.assets) { openResult(row); }
            else if (event.key === "PageDown" && state.page < state.pageCount) { search(state.page + 1); }
            else if (event.key === "PageUp" && state.page > 1) { search(state.page - 1); }
            else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                const rows = Array.from(state.results.values()).filter(item => !item.closest(".fn__none"));
                const next = rows[rows.indexOf(row) + (event.key === "ArrowDown" ? 1 : -1)];
                if (next) { showPreview(next); }
            } else { return; }
            event.preventDefault();
            event.stopPropagation();
        });
    }
    current.input.value = keyword ?? "";
    switchPanel(current);
}
