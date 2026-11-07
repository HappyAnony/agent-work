/// <reference types="./src/types" />
export declare const getDefaultOperatorByType: (type: TAVCol) => "=" | "Is false" | "Contains";
export declare const setFilter: (options: {
    filter: IAVFilter;
    protyle: IProtyle;
    data: IAV;
    target: HTMLElement;
    blockElement: Element;
    empty: boolean;
}) => Promise<void>;
export declare const addFilter: (options: {
    data: IAV;
    rect: DOMRect;
    menuElement: HTMLElement;
    tabRect: DOMRect;
    avId: string;
    protyle: IProtyle;
    blockElement: Element;
}) => void;
export declare const getFiltersHTML: (data: IAV) => string;
