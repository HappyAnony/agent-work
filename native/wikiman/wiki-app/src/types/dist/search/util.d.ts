/// <reference types="./src/types" />
import { Protyle } from "../protyle";
import { App } from "../index";
export declare const openGlobalSearch: (app: App, text: string, replace: boolean, searchData?: Config.IUILayoutTabSearchConfig) => void;
export declare const genSearch: (app: App, config: Config.IUILayoutTabSearchConfig, element: HTMLElement, closeCB?: () => void) => {
    edit: Protyle;
    unRefEdit: Protyle;
};
export declare const openSearchEditor: (options: {
    protyle: IProtyle;
    openPosition?: string;
    id: string;
    rootId: string;
    cb: () => void;
}) => void;
export declare const genQueryHTML: (method: number, id: string) => string;
export declare const updateConfig: (element: Element, item: Config.IUILayoutTabSearchConfig, config: Config.IUILayoutTabSearchConfig, edit: Protyle, clear?: boolean) => Config.IUILayoutTabSearchConfig;
export declare const getArticle: (options: {
    id: string;
    config?: Config.IUILayoutTabSearchConfig;
    edit: Protyle;
    value?: string;
}) => void;
export declare const replace: (element: Element, config: Config.IUILayoutTabSearchConfig, edit: Protyle, isAll: boolean) => void;
export declare const inputEvent: (element: Element, config: Config.IUILayoutTabSearchConfig, edit: Protyle, rmCurrentCriteria?: boolean, focusId?: {
    currentId?: string;
    newId?: string;
}) => void;
export declare const getAttr: (block: IBlock) => string;
