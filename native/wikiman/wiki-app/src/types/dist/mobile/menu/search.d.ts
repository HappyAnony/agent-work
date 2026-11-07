import { App } from "../../index";
export declare const updateSearchResult: (config: Config.IUILayoutTabSearchConfig, element: Element, rmCurrentCriteria?: boolean, focusId?: {
    currentId?: string;
    newId?: string;
}) => void;
export declare const popSearch: (app: App, searchConfig?: Config.IUILayoutTabSearchConfig) => void;
export declare const goUnRef: () => void;
