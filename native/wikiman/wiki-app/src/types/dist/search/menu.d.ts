export declare const filterMenu: (config: Config.IUILayoutTabSearchConfig, cb: () => void) => void;
export declare const replaceFilterMenu: (config: Config.IUILayoutTabSearchConfig) => void;
export declare const queryMenu: (config: Config.IUILayoutTabSearchConfig, cb: () => void) => void;
export declare const saveCriterion: (config: Config.IUILayoutTabSearchConfig, criteriaData: Config.IUILayoutTabSearchConfig[], element: Element) => void;
export declare const moreMenu: (config: Config.IUILayoutTabSearchConfig, criteriaData: Config.IUILayoutTabSearchConfig[], element: Element, cb: () => void, removeCriterion: () => void, layoutMenu?: () => void) => Promise<void>;
export declare const initCriteriaMenu: (element: HTMLElement, data: Config.IUILayoutTabSearchConfig[], config: Config.IUILayoutTabSearchConfig) => void;
export declare const getKeyByLiElement: (element: HTMLElement) => string;
