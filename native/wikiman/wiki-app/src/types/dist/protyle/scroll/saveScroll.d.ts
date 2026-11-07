export declare const saveScroll: (protyle: IProtyle, getObject?: boolean) => Promise<unknown> | IScrollAttr;
export declare const getDocByScroll: (options: {
    protyle: IProtyle;
    scrollAttr?: IScrollAttr;
    mergedOptions?: IProtyleOptions;
    cb?: (keys: string[]) => void;
    focus?: boolean;
    updateReadonly?: boolean;
}) => void;
