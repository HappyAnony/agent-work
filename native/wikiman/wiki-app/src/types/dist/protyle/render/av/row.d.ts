/// <reference types="./src/types" />
export declare const getFieldIdByCellElement: (cellElement: Element, viewType: TAVView) => string;
export declare const selectRow: (checkElement: Element, type: "toggle" | "select" | "unselect" | "unselectAll") => void;
export declare const updateHeader: (rowElement: HTMLElement) => void;
export declare const setPage: (blockElement: Element) => void;
/**
 * 前端插入一假行
 * @param options.protyle
 * @param options.blockElement
 * @param options.srcIDs
 * @param options.previousId
 * @param options.avId 存在为新增否则为拖拽插入
 */
export declare const insertAttrViewBlockAnimation: (options: {
    protyle: IProtyle;
    blockElement: Element;
    srcIDs: string[];
    previousId: string;
    groupID?: string;
}) => void;
export declare const stickyRow: (blockElement: HTMLElement, elementRect: DOMRect, status: "top" | "bottom" | "all") => void;
export declare const setPageSize: (options: {
    target: HTMLElement;
    protyle: IProtyle;
    avID: string;
    nodeElement: Element;
}) => void;
export declare const deleteRow: (blockElement: HTMLElement, protyle: IProtyle) => void;
export declare const insertRows: (options: {
    blockElement: HTMLElement;
    protyle: IProtyle;
    count: number;
    previousID: string;
    groupID?: string;
}) => void;
