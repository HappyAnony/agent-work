/// <reference types="./src/types" />
import { Tab } from "./Tab";
import { App } from "../index";
export declare class Model {
    ws: WebSocket;
    reqId: number;
    parent: Tab;
    parent: any;
    app: App;
    constructor(options: {
        app: App;
        id: string;
        type?: TWS;
        callback?: () => void;
        msgCallback?: (data: IWebSocketData) => void;
    });
    private connect;
    send(cmd: string, param: Record<string, unknown>, process?: boolean): void;
}
