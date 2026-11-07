/// <reference types="./src/types" />
import { Tab } from "../Tab";
import { Model } from "../Model";
import { App } from "../../index";
export declare class Graph extends Model {
    inputElement: HTMLInputElement;
    private graphElement;
    private panelElement;
    private element;
    private network;
    blockId: string;
    rootId: string;
    graphData: {
        nodes: {
            box: string;
            id: string;
            path: string;
            type: string;
            color: IObject;
        }[];
        links: Record<string, unknown>[];
        box: string;
    };
    type: "local" | "pin" | "global";
    constructor(options: {
        app: App;
        tab: Tab;
        blockId?: string;
        rootId?: string;
        type: "local" | "pin" | "global";
    });
    private reset;
    searchGraph(focus: boolean, id?: string, refresh?: boolean): void;
    private hlNode;
    destroy(): void;
    onGraph(hl: boolean): void;
}
