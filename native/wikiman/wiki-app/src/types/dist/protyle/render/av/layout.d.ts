/// <reference types="./src/types" />
export declare const getLayoutHTML: (data: IAV) => string;
export declare const bindLayoutEvent: (options: {
    protyle: IProtyle;
    data: IAV;
    menuElement: HTMLElement;
    blockElement: Element;
}) => void;
export declare const updateLayout: (options: {
    data: IAV;
    nodeElement: Element;
    protyle: IProtyle;
    target: HTMLElement;
}) => Promise<any>;
