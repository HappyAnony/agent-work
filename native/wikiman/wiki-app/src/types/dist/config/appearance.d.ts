export declare const appearance: {
    element: Element;
    genHTML: () => string;
    _send: (statusBar?: Config.IAppearanceStatusBar) => void;
    bindEvent: () => void;
    onSetAppearance(data: Config.IAppearance): void;
};
