export declare const initStatus: (isWindow?: boolean) => void;
export declare const countSelectWord: (range: Range, rootID?: string) => void;
export declare const countBlockWord: (ids: string[], rootID?: string, clearCache?: boolean) => void;
export declare const clearCounter: () => void;
export declare const renderStatusbarCounter: (stat: {
    runeCount: number;
    wordCount: number;
    linkCount: number;
    imageCount: number;
    refCount: number;
    blockCount: number;
}) => void;
