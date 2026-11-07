import {blockRender} from "./render/blockRender";
import {processRender} from "./util/processCode";
import {scopedDiagramSelector} from "./render/scopedSVG";
import {Constants} from "../constants";
import {Hint} from "./hint";
import {setLute} from "./render/setLute";
import {Preview} from "./preview";
import {addLoading, initUI, removeLoading} from "./ui/initUI";
import {Undo} from "./undo";
import {Upload} from "./upload";
import {Options} from "./util/Options";
import {destroy} from "./util/destroy";
import {Scroll} from "./scroll";
/// #if !SINGLE
import {Model} from "../layout/Model";
/// #endif
import {genUUID} from "../util/genID";
import {WYSIWYG} from "./wysiwyg";
import {Toolbar} from "./toolbar";
import {Gutter} from "./gutter";
import {Breadcrumb} from "./breadcrumb";
import {
    onTransaction,
    transaction,
    turnsIntoOneTransaction,
    turnsIntoTransaction,
    updateBatchTransaction,
    updateTransaction
} from "./wysiwyg/transaction";
import {fetchPost} from "../util/fetch";
/// #if !MOBILE
/// #if !SINGLE
import {updatePanelByEditor} from "../editor/util";
import {setPanelFocus} from "../layout/util";
/// #endif
/// #endif
import {Title} from "./header/Title";
import {Background} from "./header/Background";
import {disabledProtyle, enableProtyle, onGet, setReadonlyByConfig} from "./util/onGet";
import {reloadProtyle} from "./util/reload";
import {renderBacklink} from "./wysiwyg/renderBacklink";
import {setEmpty} from "../mobile/util/setEmpty";
import {resize} from "./util/resize";
import {getDocByScroll, saveScroll} from "./scroll/saveScroll";
import type {App} from "../index";
import {insertHTML} from "./util/insertHTML";
import {avRender} from "./render/av/render";
import {focusBlock, focusByOffset, getEditorRange} from "./util/selection";
import {hasClosestBlock} from "./util/hasClosest";
import {setStorageVal} from "./util/compatibility";
import {merge} from "./util/merge";
/// #if !MOBILE
/// #if !SINGLE
import {getAllModels} from "../layout/getAll";
/// #endif
/// #endif
import {isSupportCSSHL} from "./render/searchMarkRender";
import {renderAVAttribute} from "./render/av/blockAttr";
import {setFoldById, zoomOut} from "../menus/protyle";

export class Protyle {

    public readonly version: string;
    public protyle: IProtyle;
    public portableReady: Promise<void> | null = null;

    /**
     * @param id 要挂载 Protyle 的元素或者元素 ID。
     * @param options Protyle 参数
     */
    constructor(app: App, id: HTMLElement, options?: IProtyleOptions) {
        this.version = Constants.WIKI_VERSION;
        // Session owners must not pass through the generic deep option merger.
        const {portable, ...ordinaryOptions} = options || {};
        let pluginsOptions: IProtyleOptions = ordinaryOptions;
        app.plugins.forEach(item => {
            if (item.protyleOptions) {
                pluginsOptions = merge(pluginsOptions, item.protyleOptions);
            }
        });
        const getOptions = new Options(pluginsOptions);
        const mergedOptions = getOptions.merge();
        this.protyle = {
            portable,
            getInstance: () => this,
            app,
            transactionTime: new Date().getTime(),
            id: genUUID(),
            disabled: false,
            updated: false,
            element: id,
            options: mergedOptions,
            block: {
                rootID: mergedOptions.blockId || "",
            },
            highlight: {
                mark: isSupportCSSHL() ? new Highlight() : undefined,
                markHL: isSupportCSSHL() ? new Highlight() : undefined,
                ranges: [],
                rangeIndex: 0,
                styleElement: document.createElement("style"),
            }
        };

        if (isSupportCSSHL()) {
            const styleId = genUUID();
            this.protyle.highlight.styleElement.dataset.uuid = styleId;
            this.protyle.highlight.styleElement.textContent = `.protyle-content::highlight(search-mark-${styleId}) {background-color: var(--b3-highlight-background);color: var(--b3-highlight-color);}
  .protyle-content::highlight(search-mark-hl-${styleId}) {color: var(--b3-highlight-color);background-color: var(--b3-highlight-current-background)}`;
        }

        this.protyle.hint = new Hint(this.protyle);
        if (mergedOptions.render.breadcrumb) {
            this.protyle.breadcrumb = new Breadcrumb(this.protyle);
        }
        if (mergedOptions.render.title) {
            this.protyle.title = new Title(this.protyle);
        }
        if (mergedOptions.render.background) {
            this.protyle.background = new Background(this.protyle);
        }

        this.protyle.element.innerHTML = "";
        this.protyle.element.classList.add("protyle");
        if (mergedOptions.render.breadcrumb) {
            this.protyle.element.appendChild(this.protyle.breadcrumb.element.parentElement);
        }
        this.protyle.undo = new Undo();
        this.protyle.wysiwyg = new WYSIWYG(this.protyle);
        this.protyle.toolbar = new Toolbar(this.protyle);
        this.protyle.scroll = new Scroll(this.protyle); // 不能使用 render.scroll 来判读是否初始化，除非重构后面用到的相关变量
        if (this.protyle.options.render.gutter) {
            this.protyle.gutter = new Gutter(this.protyle);
        }
        if (mergedOptions.upload.url || mergedOptions.upload.handler) {
            this.protyle.upload = new Upload();
        }

        this.init();
        if (portable) {
            const editable = new Set<HTMLElement>();
            let firstRender = true;
            portable.attach({
                element: this.protyle.wysiwyg.element,
                render: (html, path) => {
                    const doc = portable.document;
                    this.protyle.notebookId = doc.boxId;
                    this.protyle.path = path ?? `/${doc.docId}.docj`;
                    Object.assign(this.protyle.block, {id:doc.docId,rootID:doc.docId,parentID:doc.docId,showAll:true,mode:0});
                    this.protyle.wysiwyg.element.innerHTML = html;
                    this.protyle.wysiwyg.lastHTMLs = {};
                    this.protyle.observerLoad?.disconnect();
                    editable.clear();
                    this.protyle.wysiwyg.element.querySelectorAll<HTMLElement>('[contenteditable="true"]').forEach(node => editable.add(node));
                    removeLoading(this.protyle);
                    blockRender(this.protyle, this.protyle.wysiwyg.element);
                    processRender(this.protyle.wysiwyg.element);
                    void avRender(this.protyle.wysiwyg.element, this.protyle).catch(error => {
                        this.protyle.wysiwyg.element.dataset.avPreviewError = error instanceof Error ? error.message : "av_preview_failed";
                    });
                    if (firstRender) {
                        firstRender = false;
                        const position = window.siyuan.storage[Constants.LOCAL_FILEPOSITION]?.[doc.docId] as IScrollAttr;
                        if (position?.rootId === doc.docId) {
                            const root = this.protyle.wysiwyg.element;
                            const block = Array.from(root.querySelectorAll<HTMLElement>("[data-node-id]")).find(node =>
                                node.dataset.nodeId === position.focusId && !node.closest(".protyle-wysiwyg__embed"));
                            if (block && Number.isInteger(position.focusStart) && Number.isInteger(position.focusEnd)) {
                                const range = focusByOffset(block, position.focusStart, position.focusEnd, false);
                                if (range) {
                                    this.protyle.toolbar.range = range;
                                    const selection = root.ownerDocument.getSelection();
                                    selection?.removeAllRanges();
                                    selection?.addRange(range);
                                }
                            }
                            this.protyle.contentElement.scrollTop = position.scrollTop || 0;
                        }
                    }
                },
                setDisabled: disabled => {
                    disabled = disabled || window.siyuan.config.readonly || window.siyuan.config.editor.readOnly ||
                        this.protyle.wysiwyg.element.getAttribute(Constants.CUSTOM_SY_READONLY) === "true";
                    this.protyle.disabled = disabled;
                    if (disabled) { this.protyle.wysiwyg.element.querySelectorAll<HTMLElement>('[contenteditable="true"]').forEach(node => editable.add(node)); }
                    editable.forEach(node => {if (this.protyle.wysiwyg.element.contains(node)) {node.setAttribute("contenteditable",disabled ? "false" : "true");}});
                    this.protyle.wysiwyg.element.querySelectorAll(
                        `${scopedDiagramSelector} .protyle-action__edit`,
                    ).forEach(icon=>icon.classList.toggle("fn__none",disabled));
                },
                clearHistory: () => this.protyle.undo.clear(),
                historyCommand: redo => {
                    if (redo) { this.protyle.undo.redo(this.protyle); }
                    else { this.protyle.undo.undo(this.protyle); }
                },
                hasOverlayDraft: () => !!this.protyle.toolbar.subElementCloseCB &&
                    !this.protyle.toolbar.subElement.classList.contains("fn__none"),
                savePosition: async () => {
                    const position = saveScroll(this.protyle, true) as IScrollAttr | undefined;
                    if (position?.rootId === portable.document.docId) {
                        await portable.session.requestUI("/api/storage/setLocalStorageVal", {
                            key: Constants.LOCAL_FILEPOSITION, entryID: position.rootId, val: position,
                        });
                    }
                },
                error: error => {
                    let message = this.protyle.element.querySelector<HTMLElement>("[data-portable-error]");
                    if (!message) {message=document.createElement("div");message.setAttribute("data-portable-error","");message.setAttribute("role","alert");this.protyle.element.appendChild(message);}
                    message.textContent = error instanceof Error ? error.message : "Portable editor failed";
                },
            });
            this.getDoc(mergedOptions);
            return; // No legacy WebSocket, global transaction queue or getConf.
        }
        if (!mergedOptions.action.includes(Constants.CB_GET_HISTORY)) {
            /// #if SINGLE
            this.protyle.ws = window.siyuan.ws;
            /// #else
            this.protyle.ws = new Model({
                app,
                id: this.protyle.id,
                type: "protyle",
                msgCallback: (data) => {
                    switch (data.cmd) {
                        case "reload":
                            if (data.data === this.protyle.block.rootID) {
                                reloadProtyle(this.protyle, false);
                                /// #if !MOBILE
                                /// #if !SINGLE
                                getAllModels().outline.forEach(item => {
                                    if (item.blockId === data.data) {
                                        fetchPost("/api/outline/getDocOutline", {
                                            id: item.blockId,
                                            preview: item.isPreview
                                        }, response => {
                                            item.update(response);
                                        });
                                    }
                                });
                                /// #endif
                                /// #endif
                            }
                            break;
                        case "refreshAttributeView":
                            Array.from(this.protyle.wysiwyg.element.querySelectorAll(`.av[data-av-id="${data.data.id}"]`)).forEach((item: HTMLElement) => {
                                item.removeAttribute("data-render");
                                avRender(item, this.protyle);
                            });
                            break;
                        case "addLoading":
                            if (data.data === this.protyle.block.rootID) {
                                addLoading(this.protyle, data.msg);
                            }
                            break;
                        case "unfoldHeading":
                            setFoldById(data.data, this.protyle);
                            break;
                        case "transactions":
                            this.onTransaction(data);
                            break;
                        case "readonly":
                            window.siyuan.config.editor.readOnly = data.data;
                            setReadonlyByConfig(this.protyle, true);
                            break;
                        case "heading2doc":
                        case "li2doc":
                            if (this.protyle.block.rootID === data.data.srcRootBlockID) {
                                if (this.protyle.block.showAll && data.cmd === "heading2doc" && !this.protyle.options.backlinkData) {
                                    fetchPost("/api/filetree/getDoc", {
                                        id: this.protyle.block.rootID,
                                        size: window.siyuan.config.editor.dynamicLoadBlocks,
                                    }, getResponse => {
                                        onGet({data: getResponse, protyle: this.protyle});
                                    });
                                } else {
                                    reloadProtyle(this.protyle, false);
                                }
                                /// #if !MOBILE
                                /// #if !SINGLE
                                if (data.cmd === "heading2doc") {
                                    // 文档标题互转后，需更新大纲
                                    updatePanelByEditor({
                                        protyle: this.protyle,
                                        focus: false,
                                        pushBackStack: false,
                                        reload: true,
                                        resize: false
                                    });
                                }
                                /// #endif
                                /// #endif
                            }
                            break;
                        case "rename":
                            if (this.protyle.path === data.data.path) {
                                if (this.protyle.model) {
                                    this.protyle.model.parent.updateTitle(data.data.title);
                                }
                                if (this.protyle.background) {
                                    this.protyle.background.ial.title = data.data.title;
                                }
                            }
                            if (this.protyle.options.render.title && this.protyle.block.parentID === data.data.id) {
                                if (!document.body.classList.contains("body--blur") && getSelection().rangeCount > 0 &&
                                    this.protyle.title.editElement?.contains(getSelection().getRangeAt(0).startContainer)) {
                                    // 标题编辑中的不用更新 https://github.com/siyuan-note/siyuan/issues/6565
                                } else {
                                    this.protyle.title.setTitle(data.data.title);
                                }
                            }
                            // update ref
                            this.protyle.wysiwyg.element.querySelectorAll(`[data-type~="block-ref"][data-id="${data.data.id}"]`).forEach(item => {
                                if (item.getAttribute("data-subtype") === "d") {
                                    // 同 updateRef 一样处理 https://github.com/siyuan-note/siyuan/issues/10458
                                    item.innerHTML = data.data.refText;
                                }
                            });
                            break;
                        case "moveDoc":
                            if (this.protyle.path === data.data.fromPath) {
                                this.protyle.path = data.data.newPath;
                                this.protyle.notebookId = data.data.toNotebook;
                            }
                            break;
                        case "unmount":
                            if (this.protyle.notebookId === data.data.box) {
                                /// #if MOBILE
                                setEmpty(app);
                                /// #else
                                if (this.protyle.model) {
                                    this.protyle.model.parent.parent.removeTab(this.protyle.model.parent.id, false);
                                }
                                /// #endif
                            }
                            break;
                        case "removeDoc":
                            if (data.data.ids.includes(this.protyle.block.rootID)) {
                                /// #if MOBILE
                                setEmpty(app);
                                /// #else
                                if (this.protyle.model) {
                                    this.protyle.model.parent.parent.removeTab(this.protyle.model.parent.id, false);
                                }
                                /// #endif
                                delete window.siyuan.storage[Constants.LOCAL_FILEPOSITION][this.protyle.block.rootID];
                                setStorageVal(Constants.LOCAL_FILEPOSITION, window.siyuan.storage[Constants.LOCAL_FILEPOSITION]);
                            }
                            break;
                    }
                }
            });
            /// #endif
            if (options.backlinkData) {
                this.protyle.block.rootID = options.blockId;
                renderBacklink(this.protyle, options.backlinkData);
                // 为了满足 eventPath0.style.paddingLeft 从而显示块标 https://github.com/siyuan-note/siyuan/issues/11578
                this.protyle.wysiwyg.element.style.padding = "4px 16px 4px 24px";
                return;
            }
            if (!options.blockId) {
                // 搜索页签需提前初始化
                removeLoading(this.protyle);
                return;
            }

            if (this.protyle.options.mode !== "preview" &&
                options.rootId && window.siyuan.storage[Constants.LOCAL_FILEPOSITION][options.rootId] &&
                (
                    mergedOptions.action.includes(Constants.CB_GET_SCROLL) ||
                    (mergedOptions.action.includes(Constants.CB_GET_ROOTSCROLL) && options.rootId === options.blockId)
                )
            ) {
                getDocByScroll({
                    protyle: this.protyle,
                    scrollAttr: window.siyuan.storage[Constants.LOCAL_FILEPOSITION][options.rootId],
                    mergedOptions,
                    cb: () => {
                        this.afterOnGet(mergedOptions);
                    }
                });
            } else {
                this.getDoc(mergedOptions);
            }
        } else {
            this.protyle.contentElement.classList.add("protyle-content--transition");
        }
    }

    private onTransaction(data: IWebSocketData) {
        let needCreateAction = "";
        data.data[0].doOperations.find((item: IOperation) => {
            if (!this.protyle.preview.element.classList.contains("fn__none")) {
                this.protyle.preview.render(this.protyle);
                if (item.action === "updateAttrs") {
                    onTransaction(this.protyle, item, false);
                }
            } else if (this.protyle.options.backlinkData && ["delete", "move"].includes(item.action)) {
                // 只对特定情况刷新，否则展开、编辑等操作刷新会频繁
                /// #if !MOBILE
                /// #if !SINGLE
                getAllModels().backlink.find(backlinkItem => {
                    if (backlinkItem.element.contains(this.protyle.element)) {
                        backlinkItem.refresh();
                        return true;
                    }
                });
                /// #endif
                /// #endif
                return true;
            } else {
                onTransaction(this.protyle, item, false);
                // 反链面板移除元素后，文档为空
                if (!(item.action === "delete" && typeof item.data?.createEmptyParagraph === "boolean" && !item.data.createEmptyParagraph)) {
                    needCreateAction = item.action;
                }
            }
        });
        if (this.protyle.wysiwyg.element.childElementCount === 0 && this.protyle.block.parentID && needCreateAction) {
            if (needCreateAction === "delete" && this.protyle.block.showAll) {
                if (this.protyle.options.handleEmptyContent) {
                    this.protyle.options.handleEmptyContent();
                } else {
                    zoomOut({
                        protyle: this.protyle,
                        id: this.protyle.block.rootID,
                        focusId: this.protyle.block.id
                    });
                }
            } else {
                // 不能使用 transaction，否则分屏后会重复添加
                this.protyle.undo.clear();
                this.reload(false);
            }
        }
    }

    private getDoc(mergedOptions: IProtyleOptions) {
        if (this.protyle.portable) {
            this.portableReady=this.protyle.portable.load().then(() => this.afterOnGet(mergedOptions));
            void this.portableReady.catch(() => undefined);
            return;
        }
        fetchPost("/api/filetree/getDoc", {
            id: mergedOptions.blockId,
            isBacklink: mergedOptions.action.includes(Constants.CB_GET_BACKLINK),
            originalRefBlockIDs: mergedOptions.originalRefBlockIDs,
            // 0: 仅当前 ID（默认值），1：向上 2：向下，3：上下都加载，4：加载最后
            mode: (mergedOptions.action && mergedOptions.action.includes(Constants.CB_GET_CONTEXT)) ? 3 : 0,
            size: mergedOptions.action?.includes(Constants.CB_GET_ALL) ? Constants.SIZE_GET_MAX : window.siyuan.config.editor.dynamicLoadBlocks,
        }, getResponse => {
            onGet({
                data: getResponse,
                protyle: this.protyle,
                action: mergedOptions.action,
                scrollPosition: mergedOptions.scrollPosition,
                afterCB: () => {
                    this.afterOnGet(mergedOptions);
                }
            });
        });
    }

    private afterOnGet(mergedOptions: IProtyleOptions) {
        if (this.protyle.model) {
            /// #if !MOBILE
            /// #if !SINGLE
            if (mergedOptions.action?.includes(Constants.CB_GET_FOCUS) || mergedOptions.action?.includes(Constants.CB_GET_OPENNEW)) {
                setPanelFocus(this.protyle.model.element.parentElement.parentElement);
            }
            updatePanelByEditor({
                protyle: this.protyle,
                focus: false,
                pushBackStack: false,
                reload: false,
                resize: false
            });
            /// #endif
            /// #endif
        }
        resize(this.protyle);   // 需等待 fullwidth 获取后设定完毕再重新计算 padding 和元素
        // 需等待 getDoc 完成后再执行，否则在无页签的时候 updatePanelByEditor 会执行2次
        // 只能用 focusin，否则点击表格无法执行
        this.protyle.wysiwyg.element.addEventListener("focusin", () => {
            /// #if !MOBILE
            /// #if !SINGLE
            if (this.protyle && this.protyle.model) {
                let needUpdate = true;
                if (this.protyle.model.element.parentElement.parentElement.classList.contains("layout__wnd--active") && this.protyle.model.headElement.classList.contains("item--focus")) {
                    needUpdate = false;
                }
                if (!needUpdate) {
                    return;
                }
                setPanelFocus(this.protyle.model.element.parentElement.parentElement);
                updatePanelByEditor({
                    protyle: this.protyle,
                    focus: false,
                    pushBackStack: false,
                    reload: false,
                    resize: false,
                });
            } else {
                // 悬浮层应移除其余面板高亮，否则按键会被面板监听到
                document.querySelectorAll(".layout__tab--active").forEach(item => {
                    item.classList.remove("layout__tab--active");
                });
                document.querySelectorAll(".layout__wnd--active").forEach(item => {
                    item.classList.remove("layout__wnd--active");
                });
            }
            /// #endif
            /// #endif
        });
        // 需等渲染完后再回调，用于定位搜索字段 https://github.com/siyuan-note/siyuan/issues/3171
        if (mergedOptions.after) {
            mergedOptions.after(this);
        }
        this.protyle.contentElement.classList.add("protyle-content--transition");
    }

    private init() {
        this.protyle.lute = setLute({
            emojiSite: this.protyle.options.hint.emojiPath,
            emojis: this.protyle.options.hint.emoji,
            headingAnchor: false,
            listStyle: this.protyle.options.preview.markdown.listStyle,
            paragraphBeginningSpace: this.protyle.options.preview.markdown.paragraphBeginningSpace,
            sanitize: this.protyle.options.preview.markdown.sanitize,
        });

        this.protyle.preview = new Preview(this.protyle);

        initUI(this.protyle);
    }

    /** 聚焦到编辑器 */
    public focus() {
        this.protyle.wysiwyg.element.focus();
    }

    /** 上传是否还在进行中 */
    public isUploading() {
        return this.protyle.upload.isUploading;
    }

    /** 清空 undo & redo 栈 */
    public clearStack() {
        this.protyle.undo.clear();
    }

    /** 销毁编辑器 */
    public destroy() {
        destroy(this.protyle);
    }

    public resize() {
        resize(this.protyle);
    }

    public reload(focus: boolean) {
        reloadProtyle(this.protyle, focus);
    }

    public insert(html: string, isBlock = false, useProtyleRange = false) {
        insertHTML(html, this.protyle, isBlock, useProtyleRange);
    }

    public transaction(doOperations: IOperation[], undoOperations?: IOperation[]) {
        transaction(this.protyle, doOperations, undoOperations);
    }

    /**
     * 多个块转换为一个块
     * @param {TTurnIntoOneSub} [subType] type 为 "BlocksMergeSuperBlock" 时必传
     */
    public turnIntoOneTransaction(selectsElement: Element[], type: TTurnIntoOne, subType?: TTurnIntoOneSub) {
        turnsIntoOneTransaction({
            protyle: this.protyle,
            selectsElement,
            type,
            level: subType
        });
    }

    /**
     * 多个块转换
     * @param {Element} [nodeElement] 优先使用包含 protyle-wysiwyg--select 的块，否则使用 nodeElement 单块
     * @param {number} [subType] type 为 "Blocks2Hs" 时必传
     */
    public turnIntoTransaction(nodeElement: Element, type: TTurnInto, subType?: number) {
        turnsIntoTransaction({
            protyle: this.protyle,
            nodeElement,
            type,
            level: subType,
        });
    }

    public updateTransaction(id: string, newHTML: string, html: string) {
        updateTransaction(this.protyle, id, newHTML, html);
    }

    public updateBatchTransaction(nodeElements: Element[], cb: (e: HTMLElement) => void) {
        updateBatchTransaction(nodeElements, this.protyle, cb);
    }

    public getRange(element: Element) {
        return getEditorRange(element);
    }

    public hasClosestBlock(element: Node) {
        return hasClosestBlock(element);
    }

    public focusBlock(element: Element, toStart = true) {
        return focusBlock(element, undefined, toStart);
    }

    public disable() {
        disabledProtyle(this.protyle);
    }

    public enable() {
        enableProtyle(this.protyle);
    }

    public renderAVAttribute(element: HTMLElement, id: string, cb?: (element: HTMLElement) => void) {
        renderAVAttribute(element, id, this.protyle, cb);
    }
}
