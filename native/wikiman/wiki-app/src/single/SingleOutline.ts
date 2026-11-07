import {Constants} from "../constants";
import {MenuItem} from "../menus/Menu";
import {hasClosestBlock} from "../protyle/util/hasClosest";
import {getPreviousBlock} from "../protyle/wysiwyg/getBlock";
import {fetchPost} from "../util/fetch";
import {Tree} from "../util/Tree";
import {isPortableWindow} from "../portable/boundary";
import {setStorageVal} from "../protyle/util/compatibility";

const OUTLINE_REFRESH_DELAY = 240;
const OUTLINE_COLLAPSED_KEY = "singleOutlineCollapsed";
const SCOPED_OUTLINE_COLLAPSED_KEY = "local-singleoutlinecollapsed";

interface SingleOutlineOptions {
    blockId: string;
    protyle: IProtyle;
}

type SingleOutlineTreeItem = IBlockTree & { folded?: boolean };

export class SingleOutline {
    private blockId: string;
    private element: HTMLElement;
    private headerElement: HTMLElement;
    private protyle: IProtyle;
    private refreshTimer = 0;
    private tree: Tree;
    private preFilterExpandIds: string[] | null = null;
    private observer: MutationObserver | null = null;
    private container: HTMLElement;
    private activeHeadingId = "";
    private activeHeadingTimer = 0;
    private activeHeadingPreferSelection = true;
    private readonly activeHeadingScrollHandler = () => this.scheduleActiveHeadingUpdate(false);
    private readonly activeHeadingSelectionHandler = () => this.scheduleActiveHeadingUpdate(true);

    constructor(options: SingleOutlineOptions) {
        this.blockId = options.blockId;
        this.protyle = options.protyle;
        this.container = document.getElementById("singleOutline");
        if (!this.container) {
            throw new Error("Element #singleOutline not found in single template.");
        }

        this.container.classList.add("fn__flex-column", "file-tree", "sy__outline");
        this.container.innerHTML = `<div class="block__icons fn__hidescrollbar">
    <span data-type="togglePanel" class="block__icon ariaLabel" aria-label="${window.siyuan.languages.min}">
        <<
    </span>
    <span class="fn__flex-1 fn__space"></span>
    <input class="b3-text-field search__label fn__none fn__size200" placeholder="${window.siyuan.languages.filterKeywordEnter}" />
    <span data-type="search" class="block__icon ariaLabel" aria-label="${window.siyuan.languages.filter}">
        <svg><use xlink:href="#iconFilter"></use></svg>
    </span>
    <span class="fn__space"></span>
    <span data-type="expandLevel" class="block__icon ariaLabel" aria-label="${window.siyuan.languages.expandLevel}">
        <svg><use xlink:href="#iconList"></use></svg>
    </span>
    <span class="fn__space"></span>
    <span data-type="expand" class="block__icon ariaLabel" aria-label="${window.siyuan.languages.expandAll}">
        <svg><use xlink:href="#iconExpand"></use></svg>
    </span>
    <span class="fn__space"></span>
    <span data-type="collapse" class="block__icon ariaLabel" aria-label="${window.siyuan.languages.foldAll}">
        <svg><use xlink:href="#iconContract"></use></svg>
    </span>
</div>
<div class="fn__flex-1" style="padding: 3px 0 8px"></div>`;
        this.headerElement = this.container.firstElementChild as HTMLElement;
        this.element = this.container.lastElementChild as HTMLElement;
        this.tree = new Tree({
            element: this.element,
            data: null,
            click: (element: HTMLElement) => {
                const id = element.getAttribute("data-node-id");
                if (id) {
                    this.scrollToHeading(id);
                }
            },
            toggleClick: (liElement) => {
                this.toggleOutlineNode(liElement);
            },
        });

        this.bindHeaderEvents();
        this.bindEditorObserver();
        this.bindActiveHeadingEvents();
        this.applyCollapsedState(this.readCollapsedState(), false);
        this.refresh();
    }

    public refreshSoon(): void {
        window.clearTimeout(this.refreshTimer);
        this.refreshTimer = window.setTimeout(() => this.refresh(), OUTLINE_REFRESH_DELAY);
    }

    public handleWsMessage(data: IWebSocketData): void {
        if (!data) {
            return;
        }
        if (data.cmd === "reload" && data.data === this.blockId) {
            this.refreshSoon();
            return;
        }
        if (data.cmd === "transactions") {
            const transactions = Array.isArray(data.data) ? data.data : [];
            if (transactions.some((transactionItem) => this.transactionTouchesOutline(transactionItem))) {
                this.refreshSoon();
            }
        }
    }

    public destroy(): void {
        window.clearTimeout(this.refreshTimer);
        window.clearTimeout(this.activeHeadingTimer);
        this.unbindActiveHeadingEvents();
        this.observer?.disconnect();
        this.observer = null;
        this.container.classList.remove("fn__flex-column", "file-tree", "sy__outline", "single-outline--collapsed");
        this.container.innerHTML = "";
    }

    private bindHeaderEvents(): void {
        const inputElement = this.headerElement.querySelector("input.b3-text-field.search__label") as HTMLInputElement;
        inputElement.addEventListener("blur", () => {
            inputElement.classList.add("fn__none");
            const filterIconElement = inputElement.nextElementSibling as HTMLElement;
            const value = inputElement.value;
            if (value) {
                filterIconElement.classList.add("block__icon--active");
                filterIconElement.setAttribute("aria-label", `${window.siyuan.languages.filter} ${value}`);
            } else {
                filterIconElement.classList.remove("block__icon--active");
                filterIconElement.setAttribute("aria-label", window.siyuan.languages.filter);
            }
            if (inputElement.dataset.value !== value) {
                inputElement.dataset.value = value;
                this.setFilter();
            }
        });
        inputElement.addEventListener("keydown", (event: KeyboardEvent) => {
            if (!event.isComposing && event.key === "Enter") {
                inputElement.dataset.value = inputElement.value;
                this.setFilter();
            }
        });
        this.headerElement.addEventListener("click", (event: MouseEvent & { target: HTMLElement }) => {
            const iconElement = event.target.closest(".block__icon") as HTMLElement | null;
            if (!iconElement) {
                return;
            }
            const type = iconElement.getAttribute("data-type");
            if (type === "search") {
                inputElement.classList.remove("fn__none");
                inputElement.select();
                return;
            }
            if (type === "expand") {
                this.tree.expandAll();
                return;
            }
            if (type === "collapse") {
                this.tree.collapseAll();
                return;
            }
            if (type === "expandLevel") {
                this.showExpandLevelMenu(iconElement);
                return;
            }
            if (type === "togglePanel") {
                this.applyCollapsedState(!this.container.classList.contains("single-outline--collapsed"));
            }
        });
    }

    private readCollapsedState(): boolean {
        return isPortableWindow() && (window.matchMedia("(max-width: 620px)").matches || window.siyuan.storage[SCOPED_OUTLINE_COLLAPSED_KEY] === true);
    }

    private applyCollapsedState(collapsed: boolean, persist = true): void {
        if (persist && isPortableWindow()) {
            window.siyuan.storage[SCOPED_OUTLINE_COLLAPSED_KEY] = collapsed;
            setStorageVal(SCOPED_OUTLINE_COLLAPSED_KEY, collapsed);
        } else if (persist) {
            localStorage.setItem(OUTLINE_COLLAPSED_KEY, String(collapsed));
        }
        if (collapsed) {
            this.headerElement
                .querySelector("input.b3-text-field.search__label")
                ?.classList.add("fn__none");
        }
        this.container.classList.toggle("single-outline--collapsed", collapsed);
        const toggleElement = this.headerElement.querySelector('[data-type="togglePanel"]') as HTMLElement | null;
        if (!toggleElement) {
            return;
        }
        toggleElement.setAttribute(
            "aria-label",
            collapsed ? window.siyuan.languages.expand : window.siyuan.languages.min,
        );
        toggleElement.textContent = collapsed ? "≡" : "<<";
    }

    private bindEditorObserver(): void {
        this.observer = new MutationObserver((mutations) => {
            const shouldRefresh = mutations.some((mutation) => {
                if (mutation.type === "attributes") {
                    return mutation.attributeName === "data-type" || mutation.attributeName === "data-subtype";
                }
                return Array.from(mutation.addedNodes).concat(Array.from(mutation.removedNodes)).some((node) => {
                    return node instanceof HTMLElement && (
                        node.matches('[data-type="NodeHeading"]') ||
                        !!node.querySelector('[data-type="NodeHeading"]')
                    );
                });
            });
            if (shouldRefresh) {
                this.refreshSoon();
            }
        });
        this.observer.observe(this.protyle.wysiwyg.element, {
            attributes: true,
            attributeFilter: ["data-type", "data-subtype"],
            childList: true,
            subtree: true,
        });
    }

    private bindActiveHeadingEvents(): void {
        this.protyle.contentElement?.addEventListener("scroll", this.activeHeadingScrollHandler, {passive: true});
        this.protyle.wysiwyg.element.addEventListener("keyup", this.activeHeadingSelectionHandler);
        this.protyle.wysiwyg.element.addEventListener("mouseup", this.activeHeadingSelectionHandler);
        this.protyle.wysiwyg.element.addEventListener("input", this.activeHeadingSelectionHandler);
        document.addEventListener("selectionchange", this.activeHeadingSelectionHandler);
    }

    private unbindActiveHeadingEvents(): void {
        this.protyle.contentElement?.removeEventListener("scroll", this.activeHeadingScrollHandler);
        this.protyle.wysiwyg.element.removeEventListener("keyup", this.activeHeadingSelectionHandler);
        this.protyle.wysiwyg.element.removeEventListener("mouseup", this.activeHeadingSelectionHandler);
        this.protyle.wysiwyg.element.removeEventListener("input", this.activeHeadingSelectionHandler);
        document.removeEventListener("selectionchange", this.activeHeadingSelectionHandler);
    }

    private scheduleActiveHeadingUpdate(preferSelection = true): void {
        this.activeHeadingPreferSelection = preferSelection;
        window.clearTimeout(this.activeHeadingTimer);
        this.activeHeadingTimer = window.setTimeout(() => this.updateActiveHeading(this.activeHeadingPreferSelection), 80);
    }

    private refresh(): void {
        fetchPost("/api/outline/getDocOutline", {
            id: this.blockId,
            preview: false,
        }, response => {
            if (response.code !== 0) {
                return;
            }
            this.update(response.data || []);
        });
    }

    private update(data: IBlockTree[]): void {
        const focusedId = this.element.querySelector(".b3-list-item--focus")?.getAttribute("data-node-id");
        const scrollTop = this.element.scrollTop;
        this.tree.updateData(this.normalizeOutlineData(data));
        const inputElement = this.headerElement.querySelector("input.b3-text-field.search__label") as HTMLInputElement;
        if (inputElement.value) {
            this.setFilter();
        }
        this.element.scrollTop = scrollTop;
        this.setActiveHeading(focusedId || this.activeHeadingId, false);
        this.scheduleActiveHeadingUpdate();
    }

    private normalizeOutlineData(data: IBlockTree[]): IBlockTree[] {
        if (!data || data.length === 0) {
            return data;
        }
        const sourceMap = new Map<string, SingleOutlineTreeItem>();
        const collect = (items: Array<IBlockTree | IBlock>) => {
            items.forEach((item: IBlockTree | IBlock) => {
                const id = item.id;
                if (id) {
                    const outlineItem = item as SingleOutlineTreeItem & IBlock & { content?: string; folded?: boolean };
                    sourceMap.set(id, {
                        box: outlineItem.box,
                        nodeType: outlineItem.nodeType || outlineItem.type,
                        hPath: outlineItem.hPath || "",
                        subType: outlineItem.subType,
                        name: (outlineItem.name || outlineItem.content || "").replace(/^H[1-6]\s*/, ""),
                        type: "outline",
                        depth: outlineItem.depth || 0,
                        id,
                        count: outlineItem.count || 0,
                        folded: outlineItem.folded,
                    });
                }
                if ("children" in item && item.children?.length) {
                    collect(item.children as Array<IBlockTree | IBlock>);
                }
                if ("blocks" in item && item.blocks?.length) {
                    collect(item.blocks as Array<IBlockTree | IBlock>);
                }
            });
        };
        collect(data);

        const headingElements = Array.from(this.protyle.wysiwyg.element.querySelectorAll('[data-type="NodeHeading"]')) as HTMLElement[];
        if (headingElements.length === 0) {
            return data;
        }

        const normalized: SingleOutlineTreeItem[] = [];
        const stack: Array<{ item: SingleOutlineTreeItem; level: number }> = [];
        headingElements.forEach((headingElement) => {
            const id = headingElement.getAttribute("data-node-id");
            const source = id ? sourceMap.get(id) : undefined;
            if (!id || !source) {
                return;
            }
            const level = this.getHeadingLevel(headingElement.getAttribute("data-subtype") || source.subType);
            const item: SingleOutlineTreeItem = {
                ...source,
                id,
                subType: source.subType || headingElement.getAttribute("data-subtype") || "",
                depth: 0,
                count: 0,
                children: undefined,
                blocks: undefined,
            };
            while (stack.length > 0 && stack[stack.length - 1].level >= level) {
                stack.pop();
            }
            const parent = stack[stack.length - 1]?.item;
            if (parent) {
                if (!parent.children) {
                    parent.children = [];
                }
                item.depth = parent.depth + 1;
                parent.children.push(item);
            } else {
                normalized.push(item);
            }
            stack.push({item, level});
        });

        if (normalized.length === 0) {
            return data;
        }
        this.resetOutlineDepth(normalized, 0);
        return normalized;
    }

    private resetOutlineDepth(items: SingleOutlineTreeItem[], depth: number): void {
        items.forEach((item) => {
            item.depth = depth;
            item.count = item.children?.length || 0;
            if (item.children?.length) {
                this.resetOutlineDepth(item.children as SingleOutlineTreeItem[], depth + 1);
            }
        });
    }

    private getHeadingLevel(subType: string): number {
        const level = parseInt((subType || "").replace("h", ""), 10);
        return Number.isNaN(level) ? 1 : level;
    }

    private scrollToHeading(id: string): void {
        const headingElement = this.protyle.wysiwyg.element.querySelector(`[data-node-id="${id}"]`) as HTMLElement | null;
        if (!headingElement) {
            return;
        }
        headingElement.scrollIntoView({block: "start"});
        this.setActiveHeading(id, true);
    }

    private updateActiveHeading(preferSelection: boolean): void {
        this.setActiveHeading(
            (preferSelection ? this.getActiveHeadingIdBySelection() : "") || this.getActiveHeadingIdByScroll(),
            true,
        );
    }

    private getActiveHeadingIdBySelection(): string {
        const selection = getSelection();
        if (!selection || selection.rangeCount === 0) {
            return "";
        }
        const startContainer = selection.getRangeAt(0).startContainer;
        if (!this.protyle.wysiwyg.element.contains(startContainer)) {
            return "";
        }
        let blockElement = hasClosestBlock(startContainer) as HTMLElement | false;
        while (blockElement) {
            if (blockElement.getAttribute("data-type") === "NodeHeading") {
                return blockElement.getAttribute("data-node-id") || "";
            }
            blockElement = getPreviousBlock(blockElement) as HTMLElement | false;
        }
        return "";
    }

    private getActiveHeadingIdByScroll(): string {
        const contentElement = this.protyle.contentElement;
        const headings = Array.from(this.protyle.wysiwyg.element.querySelectorAll('[data-type="NodeHeading"]')) as HTMLElement[];
        if (!contentElement || headings.length === 0) {
            return "";
        }
        const contentRect = contentElement.getBoundingClientRect();
        const thresholdTop = contentRect.top + 96;
        let activeId = "";
        headings.some((headingElement) => {
            if (headingElement.getBoundingClientRect().top > thresholdTop) {
                return true;
            }
            activeId = headingElement.getAttribute("data-node-id") || "";
            return false;
        });
        return activeId;
    }

    private setActiveHeading(id: string, scrollOutline: boolean): void {
        if (id === this.activeHeadingId && this.element.querySelector(`.b3-list-item[data-node-id="${id}"]`)?.classList.contains("b3-list-item--focus")) {
            return;
        }
        this.activeHeadingId = id || "";
        this.element.querySelectorAll(".b3-list-item--focus").forEach(item => {
            item.classList.remove("b3-list-item--focus");
        });
        if (!id) {
            return;
        }
        let currentElement = this.element.querySelector(`.b3-list-item[data-node-id="${id}"]`) as HTMLElement | null;
        if (!currentElement) {
            return;
        }
        let parentElement = currentElement.parentElement;
        while (parentElement && parentElement !== this.element) {
            if (parentElement.tagName === "UL") {
                parentElement.classList.remove("fn__none");
                parentElement.previousElementSibling
                    ?.querySelector(".b3-list-item__arrow")
                    ?.classList.add("b3-list-item__arrow--open");
            }
            parentElement = parentElement.parentElement;
        }
        while (currentElement && currentElement.clientHeight === 0) {
            currentElement = currentElement.parentElement?.previousElementSibling as HTMLElement | null;
        }
        if (!currentElement) {
            return;
        }
        currentElement.classList.add("b3-list-item--focus");
        if (!scrollOutline) {
            return;
        }
        const outlineRect = this.element.getBoundingClientRect();
        const itemRect = currentElement.getBoundingClientRect();
        if (itemRect.top < outlineRect.top || itemRect.bottom > outlineRect.bottom) {
            this.element.scrollTop += itemRect.top - (outlineRect.top + outlineRect.height / 2);
        }
    }

    private toggleOutlineNode(liElement: Element): void {
        if (!liElement.nextElementSibling) {
            return;
        }
        const svgElement = liElement.firstElementChild.firstElementChild;
        if (svgElement.classList.contains("b3-list-item__arrow--open")) {
            svgElement.classList.remove("b3-list-item__arrow--open");
            liElement.nextElementSibling.classList.add("fn__none");
            if (liElement.nextElementSibling.nextElementSibling && liElement.nextElementSibling.nextElementSibling.tagName === "UL") {
                liElement.nextElementSibling.nextElementSibling.classList.add("fn__none");
            }
        } else {
            svgElement.classList.add("b3-list-item__arrow--open");
            liElement.nextElementSibling.classList.remove("fn__none");
            if (liElement.nextElementSibling.nextElementSibling && liElement.nextElementSibling.nextElementSibling.tagName === "UL") {
                liElement.nextElementSibling.nextElementSibling.classList.remove("fn__none");
            }
        }
    }

    private setFilter(): void {
        this.element.querySelectorAll('li.b3-list-item[style$="display: none;"]').forEach((item: HTMLElement) => {
            item.style.display = "";
        });
        this.element.querySelectorAll("ul.fn__none").forEach((item) => {
            item.previousElementSibling.querySelector(".b3-list-item__toggle")?.classList.remove("fn__hidden");
        });
        const keyword = (this.headerElement.querySelector("input.b3-text-field.search__label") as HTMLInputElement).value.toLowerCase();
        if (keyword) {
            if (!this.preFilterExpandIds) {
                this.preFilterExpandIds = this.tree.getExpandIds();
            }
            this.filterList(this.element.firstElementChild, keyword);
            return;
        }
        this.tree.setExpandIds(this.preFilterExpandIds);
        this.preFilterExpandIds = null;
    }

    private filterList(ulElement: Element, keyword: string): { hasMatch: boolean; hasChildMatch: boolean } {
        let hasMatch = false;
        let hasChildMatch = false;
        ulElement.querySelectorAll(":scope > li.b3-list-item").forEach((liItem: HTMLElement) => {
            const nextUlElement = (liItem.nextElementSibling && liItem.nextElementSibling.tagName === "UL") ? liItem.nextElementSibling as HTMLElement : undefined;
            let childResult = {hasMatch: false, hasChildMatch: false};
            if (nextUlElement) {
                childResult = this.filterList(nextUlElement, keyword);
            }
            const arrowElement = liItem.querySelector(".b3-list-item__arrow");
            const text = (liItem.querySelector(".b3-list-item__text")?.textContent || "").trim().toLowerCase();
            if (text.includes(keyword)) {
                liItem.style.display = "";
                hasMatch = true;
                if (nextUlElement) {
                    nextUlElement.classList.toggle("fn__none", !(childResult.hasMatch || childResult.hasChildMatch));
                    arrowElement?.classList.toggle("b3-list-item__arrow--open", childResult.hasMatch || childResult.hasChildMatch);
                    arrowElement?.parentElement.classList.toggle("fn__hidden", !(childResult.hasMatch || childResult.hasChildMatch));
                }
            } else if (childResult.hasMatch || childResult.hasChildMatch) {
                liItem.style.display = "";
                hasChildMatch = true;
                if (nextUlElement) {
                    nextUlElement.classList.remove("fn__none");
                    arrowElement?.classList.add("b3-list-item__arrow--open");
                }
            } else {
                liItem.style.display = "none";
                nextUlElement?.classList.add("fn__none");
            }
        });
        return {hasMatch, hasChildMatch};
    }

    private showExpandLevelMenu(target: HTMLElement): void {
        window.siyuan.menus.menu.remove();
        window.siyuan.menus.menu.element.setAttribute("data-name", Constants.MENU_OUTLINE_EXPAND_LEVEL);
        for (let i = 1; i <= 6; i++) {
            window.siyuan.menus.menu.append(new MenuItem({
                id: `heading${i}`,
                icon: `iconH${i}`,
                label: window.siyuan.languages[`heading${i}`],
                click: () => this.expandToLevel(i),
            }).element);
        }
        const rect = target.getBoundingClientRect();
        window.siyuan.menus.menu.popup({
            x: rect.left,
            y: rect.bottom,
            h: rect.height,
        });
    }

    private expandToLevel(targetLevel: number): void {
        if (targetLevel >= 6) {
            this.tree.expandAll();
            return;
        }
        this.element.querySelectorAll("li.b3-list-item").forEach((item: HTMLElement) => {
            const headingLevel = parseInt(item.getAttribute("data-subtype")?.replace("h", "") || "0");
            const arrowElement = item.querySelector(".b3-list-item__arrow");
            if (item.nextElementSibling && item.nextElementSibling.tagName === "UL" && arrowElement) {
                if (headingLevel > 0 && headingLevel < targetLevel) {
                    arrowElement.classList.add("b3-list-item__arrow--open");
                    item.nextElementSibling.classList.remove("fn__none");
                } else if (headingLevel >= targetLevel) {
                    arrowElement.classList.remove("b3-list-item__arrow--open");
                    item.nextElementSibling.classList.add("fn__none");
                }
            }
        });
    }

    private transactionTouchesOutline(transactionItem: { doOperations?: IOperation[]; undoOperations?: IOperation[] }): boolean {
        const operations = [
            ...(transactionItem.doOperations || []),
            ...(transactionItem.undoOperations || []),
        ];
        return operations.some((operation) => {
            const operationData = typeof operation.data === "string" ? operation.data : "";
            return operation.action === "delete" ||
                operation.action === "move" ||
                operation.action === "insert" && operationData.includes('data-type="NodeHeading"') ||
                operation.action === "update" && (
                    !!this.element.querySelector(`.b3-list-item[data-node-id="${operation.id}"]`) ||
                    operationData.includes('data-type="NodeHeading"')
                );
        });
    }
}
