/// <reference types="./src/types" />
export declare const getTextStar: (blockElement: HTMLElement, contentOnly?: boolean) => string;
export declare const getPlainText: (blockElement: HTMLElement, isNested?: boolean) => string;
export declare const pasteEscaped: (protyle: IProtyle, nodeElement: Element) => Promise<void>;
export declare const pasteAsPlainText: (protyle: IProtyle) => Promise<void>;
export declare const enableLuteMarkdownSyntax: (protyle: IProtyle) => void;
export declare const restoreLuteMarkdownSyntax: (protyle: IProtyle) => void;
export declare const paste: (protyle: IProtyle, event: (ClipboardEvent | DragEvent | IClipboardData) & {
    target: HTMLElement;
}) => Promise<void>;
