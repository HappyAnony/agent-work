/// <reference types="./src/types" />
export declare const avClick: (protyle: IProtyle, event: MouseEvent & {
    target: HTMLElement;
}) => boolean;
export declare const avContextmenu: (protyle: IProtyle, rowElement: HTMLElement, position: IPosition) => boolean;
export declare const updateAVName: (protyle: IProtyle, blockElement: Element) => boolean;
export declare const updateAttrViewCellAnimation: (cellElement: HTMLElement, value: IAVCellValue, headerValue?: {
    icon?: string;
    name?: string;
    pin?: boolean;
    type?: TAVCol;
}) => void;
export declare const removeAttrViewColAnimation: (blockElement: Element, id: string) => void;
export declare const duplicateCompletely: (protyle: IProtyle, nodeElement: HTMLElement) => void;
