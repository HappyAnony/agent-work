import { Tab } from "../Tab";
import { Model } from "../Model";
import { Tree } from "../../util/Tree";
import { App } from "../../index";
export declare class Bookmark extends Model {
    private openNodes;
    tree: Tree;
    private element;
    constructor(app: App, tab: Tab);
    update(): void;
}
