/// <reference types="./src/types" />
import { App } from "../index";
export declare const genCardItem: (item: ICardPackage) => string;
export declare const makeCard: (app: App, ids: string[]) => void;
export declare const quickMakeCard: (protyle: IProtyle, nodeElement: Element[]) => void;
