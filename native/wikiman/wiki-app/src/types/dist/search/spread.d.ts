import { App } from "../index";
export declare const openSearch: (options: {
    app: App;
    hotkey: string;
    key?: string;
    notebookId?: string;
    searchPath?: string;
}) => Promise<void>;
