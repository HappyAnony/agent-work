/// <reference types="./src/types" />
import { App } from "../index";
export declare class Protyle {
    readonly version: string;
    protyle: IProtyle;
    /**
     * @param id 要挂载 Protyle 的元素或者元素 ID。
     * @param options Protyle 参数
     */
    constructor(app: App, id: HTMLElement, options?: IProtyleOptions);
    private onTransaction;
    private getDoc;
    private afterOnGet;
    private init;
    /** 聚焦到编辑器 */
    focus(): void;
    /** 上传是否还在进行中 */
    isUploading(): boolean;
    /** 清空 undo & redo 栈 */
    clearStack(): void;
    /** 销毁编辑器 */
    destroy(): void;
    resize(): void;
    reload(focus: boolean): void;
    insert(html: string, isBlock?: boolean, useProtyleRange?: boolean): void;
    transaction(doOperations: IOperation[], undoOperations?: IOperation[]): void;
    /**
     * 多个块转换为一个块
     * @param {TTurnIntoOneSub} [subType] type 为 "BlocksMergeSuperBlock" 时必传
     */
    turnIntoOneTransaction(selectsElement: Element[], type: TTurnIntoOne, subType?: TTurnIntoOneSub): void;
    /**
     * 多个块转换
     * @param {Element} [nodeElement] 优先使用包含 protyle-wysiwyg--select 的块，否则使用 nodeElement 单块
     * @param {number} [subType] type 为 "Blocks2Hs" 时必传
     */
    turnIntoTransaction(nodeElement: Element, type: TTurnInto, subType?: number): void;
    updateTransaction(id: string, newHTML: string, html: string): void;
    updateBatchTransaction(nodeElements: Element[], cb: (e: HTMLElement) => void): void;
    getRange(element: Element): Range;
    hasClosestBlock(element: Node): false | HTMLElement;
    focusBlock(element: Element, toStart?: boolean): false | Range;
    disable(): void;
    enable(): void;
    renderAVAttribute(element: HTMLElement, id: string, cb?: (element: HTMLElement) => void): void;
}
