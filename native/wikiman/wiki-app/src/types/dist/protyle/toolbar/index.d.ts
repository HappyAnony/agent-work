/// <reference types="./src/types" />
export declare class Toolbar {
    element: HTMLElement;
    subElement: HTMLElement;
    subElementCloseCB: () => void;
    range: Range;
    toolbarHeight: number;
    constructor(protyle: IProtyle);
    update(protyle: IProtyle): void;
    render(protyle: IProtyle, range: Range, event?: KeyboardEvent): void;
    getCurrentType(range?: Range): string[];
    setInlineMark(protyle: IProtyle, type: string, action: "range" | "toolbar", textObj?: ITextOption): Node[];
    showRender(protyle: IProtyle, renderElement: Element, updateElements?: Element[], oldHTML?: string): void;
    showCodeLanguage(protyle: IProtyle, languageElements: HTMLElement[]): void;
    showTpl(protyle: IProtyle, nodeElement: HTMLElement, range: Range): void;
    showWidget(protyle: IProtyle, nodeElement: HTMLElement, range: Range): void;
    showContent(protyle: IProtyle, range: Range, nodeElement: Element): void;
    private genItem;
    private mergeNode;
    private updateLanguage;
}
