/// <reference types="./src/types" />
export declare const getDateHTML: (cellElements: HTMLElement[]) => string;
export declare const bindDateEvent: (options: {
    protyle: IProtyle;
    data: IAV;
    menuElement: HTMLElement;
    blockElement: Element;
    cellElements: HTMLElement[];
}) => () => void;
