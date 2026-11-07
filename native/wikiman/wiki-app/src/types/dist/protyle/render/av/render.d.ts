/// <reference types="./src/types" />
export declare const genTabHeaderHTML: (data: IAV, showSearch: boolean, editable: boolean) => string;
export declare const getGroupTitleHTML: (group: IAVView, counter: number) => string;
export declare const avRender: (element: Element, protyle: IProtyle, cb?: (data: IAV) => void, renderAll?: boolean, avData?: IAV) => Promise<void>;
export declare const updateSearch: (e: HTMLElement, protyle: IProtyle) => void;
export declare const refreshAV: (protyle: IProtyle, operation: IOperation) => void;
