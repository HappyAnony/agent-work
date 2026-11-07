/// <reference types="./src/types" />
export declare const getPageSize: (blockElement: Element) => {
    groupPageSize: {
        [key: string]: {
            pageSize: number;
        };
    };
    unGroupPageSize: number;
};
export declare const setGroupMethod: (options: {
    protyle: IProtyle;
    fieldId: string;
    data: IAV;
    menuElement: HTMLElement;
    blockElement: Element;
}) => Promise<void>;
export declare const getGroupsMethodHTML: (columns: IAVColumn[], group: IAVGroup, viewType: TAVView) => string;
export declare const getLanguageByIndex: (index: number, type: "sort" | "date") => any;
export declare const getGroupsNumberHTML: (group: IAVGroup) => string;
export declare const bindGroupsNumber: (options: {
    protyle: IProtyle;
    menuElement: HTMLElement;
    blockElement: Element;
    data: IAV;
}) => () => Promise<void>;
export declare const getGroupsHTML: (columns: IAVColumn[], view: IAVView) => string;
export declare const bindGroupsEvent: (options: {
    protyle: IProtyle;
    menuElement: HTMLElement;
    blockElement: Element;
    data: IAV;
}) => void;
export declare const goGroupsDate: (options: {
    protyle: IProtyle;
    target: Element;
    menuElement: HTMLElement;
    data: IAV;
    blockElement: Element;
}) => void;
export declare const goGroupsSort: (options: {
    protyle: IProtyle;
    target: Element;
    data: IAV;
    menuElement: HTMLElement;
    blockElement: Element;
}) => void;
