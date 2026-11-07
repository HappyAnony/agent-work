/// <reference types="./src/types" />
export declare const openSearchAV: (avId: string, target: HTMLElement, cb?: (element: HTMLElement) => void, excludes?: boolean) => void;
export declare const updateRelation: (options: {
    protyle: IProtyle;
    avID: string;
    avElement: Element;
    colsData: IAVColumn[];
    blockElement: Element;
}) => void;
export declare const toggleUpdateRelationBtn: (menuItemsElement: HTMLElement, avId: string, resetData?: boolean) => void;
export declare const bindRelationEvent: (options: {
    menuElement: HTMLElement;
    protyle: IProtyle;
    blockElement: Element;
    cellElements: HTMLElement[];
}) => void;
export declare const getRelationHTML: (data: IAV, cellElements?: HTMLElement[]) => string;
export declare const setRelationCell: (protyle: IProtyle, nodeElement: HTMLElement, target: HTMLElement, cellElements: HTMLElement[]) => void;
