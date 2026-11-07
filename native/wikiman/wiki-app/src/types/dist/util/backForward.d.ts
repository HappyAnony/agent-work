import { App } from "../index";
export declare const goBack: (app: App) => Promise<void>;
export declare const goForward: (app: App) => Promise<void>;
export declare const pushBack: (protyle: IProtyle, range?: Range, blockElement?: Element) => void;
