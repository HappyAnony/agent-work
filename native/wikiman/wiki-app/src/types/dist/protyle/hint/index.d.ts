export declare class Hint {
    timeId: number;
    element: HTMLDivElement;
    enableSlash: boolean;
    private enableEmoji;
    enableExtend: boolean;
    splitChar: string;
    lastIndex: number;
    private source;
    constructor(protyle: IProtyle);
    render(protyle: IProtyle): void;
    genLoading(protyle: IProtyle): void;
    bindUploadEvent(protyle: IProtyle, element: HTMLElement): void;
    private getHTMLByData;
    genHTML(data: IHintData[], protyle: IProtyle, hide: boolean, source: THintSource): void;
    private genSearchHTML;
    private genEmojiHTML;
    fill(value: string, protyle: IProtyle, updateRange?: boolean, refIsS?: boolean): void;
    select(event: KeyboardEvent, protyle: IProtyle): boolean;
    private fixImageCursor;
    private getKey;
}
