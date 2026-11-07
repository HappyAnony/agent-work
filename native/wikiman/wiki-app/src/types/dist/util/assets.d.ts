export declare const loadAssets: (data: Config.IAppearance) => void;
export declare const initAssets: () => void;
export declare const setInlineStyle: (set?: boolean, servePath?: string) => Promise<string>;
export declare const setCodeTheme: (cdn?: string) => void;
export declare const setMode: (modeElementValue: number) => void;
export declare const getThemeMode: () => "dark" | "light";
