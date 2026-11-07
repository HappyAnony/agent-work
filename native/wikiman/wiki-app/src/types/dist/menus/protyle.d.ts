/// <reference types="./src/types" />
export declare const assetMenu: (protyle: IProtyle, position: IPosition, callback?: (url: string, name: string) => void, exts?: string[]) => void;
export declare const fileAnnotationRefMenu: (protyle: IProtyle, refElement: HTMLElement) => void;
export declare const refMenu: (protyle: IProtyle, element: HTMLElement) => void;
export declare const contentMenu: (protyle: IProtyle, nodeElement: Element) => void;
export declare const enterBack: (protyle: IProtyle, id: string) => void;
export declare const zoomOut: (options: {
    protyle: IProtyle;
    id: string;
    focusId?: string;
    isPushBack?: boolean;
    callback?: () => void;
    reload?: boolean;
}) => void;
export declare const imgMenu: (protyle: IProtyle, range: Range, assetElement: HTMLElement, position: {
    clientX: number;
    clientY: number;
}) => void;
export declare const linkMenu: (protyle: IProtyle, linkElement: HTMLElement, focusText?: boolean) => void;
export declare const tagMenu: (protyle: IProtyle, tagElement: HTMLElement) => void;
export declare const inlineMathMenu: (protyle: IProtyle, element: Element) => void;
export declare const iframeMenu: (protyle: IProtyle, nodeElement: Element) => IMenu[];
export declare const videoMenu: (protyle: IProtyle, nodeElement: Element, type: string) => IMenu[];
export declare const tableMenu: (protyle: IProtyle, nodeElement: Element, cellElement: HTMLTableCellElement, range: Range) => {
    menus: IMenu[];
    removeMenus: {
        id: string;
        icon: string;
        label: any;
        accelerator: string;
        click: () => void;
    }[];
    insertMenus: {
        id: string;
        icon: string;
        label: any;
        accelerator: string;
        click: () => void;
    }[];
    otherMenus: IMenu[];
    other2Menus: IMenu[];
};
export declare const setFoldById: (data: {
    id: string;
    currentNodeID: string;
}, protyle: IProtyle) => void;
export declare const setFold: (protyle: IProtyle, nodeElement: Element, isOpen?: boolean, isRemove?: boolean, addLoading?: boolean, getOperations?: boolean) => {
    fold: number;
    undoOperations?: undefined;
    doOperations?: undefined;
} | {
    fold: number;
    undoOperations: IOperation[];
    doOperations: IOperation[];
};
