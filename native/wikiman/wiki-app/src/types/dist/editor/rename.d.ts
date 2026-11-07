export declare const validateName: (name: string, targetElement?: HTMLElement) => boolean;
export declare const replaceFileName: (name: string) => string;
export declare const replaceLocalPath: (name: string) => string;
export declare const rename: (options: {
    path: string;
    notebookId: string;
    name: string;
    type: "notebook" | "file";
    range?: Range;
}) => void;
export declare const renameAsset: (assetPath: string) => void;
export declare const newFileContentBySelect: (protyle: IProtyle) => void;
