/// <reference types="./src/types" />
export declare const openViewMenu: (options: {
    protyle: IProtyle;
    blockElement: HTMLElement;
    element: HTMLElement;
}) => void;
export declare const bindViewEvent: (options: {
    protyle: IProtyle;
    data: IAV;
    menuElement: HTMLElement;
    blockElement: Element;
}) => void;
export declare const getViewHTML: (data: IAV) => string;
export declare const bindSwitcherEvent: (options: {
    protyle: IProtyle;
    menuElement: Element;
    blockElement: Element;
}) => void;
export declare const getSwitcherHTML: (views: IAVView[], viewId: string) => string;
export declare const addView: (protyle: IProtyle, blockElement: Element) => void;
export declare const getViewIcon: (type: string) => "iconTable" | "iconGallery" | "iconBoard";
export declare const getViewName: (type: string) => any;
export declare const getFieldsByData: (data: IAV) => IAVColumn[];
export declare const dragoverTab: (event: DragEvent) => void;
