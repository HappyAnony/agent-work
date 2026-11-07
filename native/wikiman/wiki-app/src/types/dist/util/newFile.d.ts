import { App } from "../index";
export declare const getNewFilePath: (useSavePath: boolean) => {
    notebookId: string;
    currentPath: string;
};
export declare const newFile: (optios: {
    app: App;
    notebookId?: string;
    currentPath?: string;
    paths?: string[];
    useSavePath: boolean;
    name?: string;
    afterCB?: (id: string, title: string) => void;
    listDocTree?: boolean;
}) => void;
export declare const getSavePath: (pathString: string, notebookId: string, cb: (p: string, notebookId: string) => void) => void;
export declare const newFileByName: (app: App, value: string) => void;
export declare const newFileBySelect: (protyle: IProtyle, selectText: string, nodeElement: HTMLElement, pathDir: string, targetNotebookId: string) => void;
