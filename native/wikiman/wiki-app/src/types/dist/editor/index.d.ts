import { Tab } from "../layout/Tab";
import { Protyle } from "../protyle";
import { Model } from "../layout/Model";
import { App } from "../index";
export declare class Editor extends Model {
    element: HTMLElement;
    editor: Protyle;
    headElement: HTMLElement;
    constructor(options: {
        app: App;
        tab: Tab;
        blockId: string;
        rootId: string;
        mode?: TEditorMode;
        action?: TProtyleAction[];
        afterInitProtyle?: (editor: Protyle) => void;
        scrollPosition?: ScrollLogicalPosition;
    });
    private initProtyle;
}
