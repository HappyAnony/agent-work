/// <reference types="./src/types" />
export declare class Background {
    element: HTMLElement;
    ial: IObject;
    private imgElement;
    private iconElement;
    private actionElements;
    private tagsElement;
    private transparentData;
    constructor(protyle: IProtyle);
    private removeTag;
    render(ial: IObject, rootId: string): void;
    private openTag;
    private getTags;
    private addTags;
}
