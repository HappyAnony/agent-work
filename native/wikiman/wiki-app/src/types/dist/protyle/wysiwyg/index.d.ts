/// <reference types="./src/types" />
export declare class WYSIWYG {
    lastHTMLs: {
        [key: string]: string;
    };
    element: HTMLDivElement;
    preventKeyup: boolean;
    private preventClick;
    constructor(protyle: IProtyle);
    renderCustom(ial: IObject): void;
    private escapeInline;
    private setEmptyOutline;
    private emojiToMd;
    private bindCommonEvent;
    private bindEvent;
}
