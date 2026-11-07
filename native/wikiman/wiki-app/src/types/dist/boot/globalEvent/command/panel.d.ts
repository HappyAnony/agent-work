import { App } from "../../../index";
export declare const commandPanel: (app: App) => void;
export declare const execByCommand: (options: {
    command: string;
    app?: App;
    previousRange?: Range;
    protyle?: IProtyle;
    fileLiElements?: Element[];
}) => Promise<boolean>;
