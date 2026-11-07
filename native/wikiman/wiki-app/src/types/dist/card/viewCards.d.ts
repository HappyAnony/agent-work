/// <reference types="./src/types" />
import { App } from "../index";
export declare const viewCards: (app: App, deckID: string, title: string, deckType: "Tree" | "" | "Notebook", cb?: (response: IWebSocketData) => void) => void;
