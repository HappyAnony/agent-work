export declare const commonHotkey: (protyle: IProtyle, event: KeyboardEvent, nodeElement?: HTMLElement) => boolean;
export declare const upSelect: (options: {
    protyle: IProtyle;
    event: KeyboardEvent;
    nodeElement: HTMLElement;
    editorElement: HTMLElement;
    range: Range;
    cb: (selectElements: NodeListOf<Element>) => void;
}) => void;
export declare const downSelect: (options: {
    protyle: IProtyle;
    event: KeyboardEvent;
    nodeElement: HTMLElement;
    editorElement: HTMLElement;
    range: Range;
    cb: (selectElement: NodeListOf<Element>) => void;
}) => void;
export declare const getStartEndElement: (selectElements: NodeListOf<Element> | Element[]) => {
    startElement: Element;
    endElement: Element;
};
export declare const duplicateBlock: (nodeElements: Element[], protyle: IProtyle) => Promise<void>;
export declare const goHome: (protyle: IProtyle) => void;
export declare const goEnd: (protyle: IProtyle) => void;
export declare const alignImgCenter: (protyle: IProtyle, nodeElement: Element, assetElements: Element[], id: string, html: string) => void;
export declare const alignImgLeft: (protyle: IProtyle, nodeElement: Element, assetElements: Element[], id: string, html: string) => void;
