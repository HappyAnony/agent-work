/// <reference types="./src/types" />
export declare const cancelSB: (protyle: IProtyle, nodeElement: Element, range?: Range) => Promise<{
    doOperations: IOperation[];
    undoOperations: IOperation[];
    previousId: string;
}>;
export declare const genSBElement: (layout: string, id?: string, attrHTML?: string) => HTMLDivElement;
export declare const jumpToParent: (protyle: IProtyle, nodeElement: Element, type: "parent" | "next" | "previous") => void;
export declare const insertEmptyBlock: (protyle: IProtyle, position: InsertPosition, id?: string) => void;
export declare const genEmptyBlock: (zwsp?: boolean, wbr?: boolean, string?: string) => string;
export declare const genEmptyElement: (zwsp?: boolean, wbr?: boolean, id?: string) => HTMLDivElement;
export declare const genHeadingElement: (headElement: Element, getHTML?: boolean, addWbr?: boolean) => string | Element;
export declare const getLangByType: (type: string) => string;
