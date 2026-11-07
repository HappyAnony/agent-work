export declare class Scroll {
    element: HTMLElement;
    private parentElement;
    private inputElement;
    lastScrollTop: number;
    keepLazyLoad: boolean;
    constructor(protyle: IProtyle);
    private setIndex;
    updateIndex(protyle: IProtyle, id: string, cb?: (index: number) => void): void;
    update(protyle: IProtyle): void;
}
