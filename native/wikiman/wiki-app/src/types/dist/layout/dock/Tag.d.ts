import { Tab } from "../Tab";
import { Model } from "../Model";
import { Tree } from "../../util/Tree";
import { App } from "../../index";
export declare class Tag extends Model {
    private openNodes;
    tree: Tree;
    private element;
    constructor(app: App, tab: Tab);
    update(ignoreMaxListHint?: boolean): void;
}
