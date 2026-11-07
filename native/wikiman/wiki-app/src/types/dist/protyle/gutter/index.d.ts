export declare class Gutter {
    element: HTMLElement;
    private gutterTip;
    constructor(protyle: IProtyle);
    isMatchNode(item: Element): boolean;
    private turnsOneInto;
    private turnsIntoOne;
    private turnsInto;
    private showMobileAppearance;
    renderMultipleMenu(protyle: IProtyle, selectsElement: Element[]): import("../../menus/Menu").Menu;
    renderMenu(protyle: IProtyle, buttonElement: Element): import("../../menus/Menu").Menu;
    private genHeadingTransform;
    private genClick;
    private genAlign;
    private updateNodeElements;
    private genWidths;
    private genHeights;
    private genCopyTextRef;
    render(protyle: IProtyle, element: Element, wysiwyg: HTMLElement, target?: Element): void;
}
