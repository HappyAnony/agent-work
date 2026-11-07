/// <reference types="./src/types" />
import { App } from "../index";
import { EventBus } from "./EventBus";
import { Custom } from "../layout/dock/Custom";
import { Tab } from "../layout/Tab";
import { MobileCustom } from "../mobile/dock/MobileCustom";
import { Setting } from "./Setting";
export declare class Plugin {
    private app;
    i18n: IObject;
    eventBus: EventBus;
    data: any;
    displayName: string;
    readonly name: string;
    protyleSlash: {
        filter: string[];
        html: string;
        id: string;
        callback: (protyle: import("../protyle").Protyle, nodeElement: HTMLElement) => void;
    }[];
    customBlockRenders: {
        [key: string]: {
            icon: string;
            action: "edit" | "more"[];
            genCursor: boolean;
            render: (options: {
                app: App;
                element: Element;
            }) => void;
        };
    };
    topBarIcons: Element[];
    setting: Setting;
    statusBarIcons: Element[];
    commands: ICommand[];
    models: {
        [key: string]: (options: {
            tab: Tab;
            data: any;
        }) => Custom;
    };
    docks: {
        [key: string]: {
            config: IPluginDockTab;
            model: (options: {
                tab: Tab;
            }) => Custom;
            mobileModel: (element: Element) => MobileCustom;
        };
    };
    private protyleOptionsValue;
    constructor(options: {
        app: App;
        name: string;
        displayName: string;
        i18n: IObject;
    });
    onload(): Promise<void> | void;
    onunload(): void;
    uninstall(): void;
    onDataChanged(): void;
    updateCards(options: ICardData): Promise<ICardData>;
    onLayoutReady(): void;
    addCommand(command: ICommand): void;
    addIcons(svg: string): void;
    addTopBar(options: {
        icon: string;
        title: string;
        position?: "south" | "left";
        callback: (evt: MouseEvent) => void;
    }): HTMLDivElement;
    addStatusBar(options: {
        element: HTMLElement;
        position?: "right" | "left";
    }): HTMLElement;
    openSetting(): void;
    loadData(storageName: string): Promise<unknown>;
    saveData(storageName: string, data: any): Promise<unknown>;
    removeData(storageName: string): Promise<unknown>;
    getOpenedTab(): {
        [key: string]: Custom[];
    };
    addTab(options: {
        type: string;
        destroy?: () => void;
        beforeDestroy?: () => void;
        resize?: () => void;
        update?: () => void;
        init: () => void;
    }): (options: {
        tab: Tab;
        data: any;
    }) => Custom;
    addDock(options: {
        config: IPluginDockTab;
        data: any;
        type: string;
        destroy?: () => void;
        resize?: () => void;
        update?: () => void;
        init: () => void;
    }): {
        config: IPluginDockTab;
        model: (options: {
            tab: Tab;
        }) => Custom;
        mobileModel: (element: Element) => MobileCustom;
    };
    addFloatLayer: (options: {
        refDefs: IRefDefs[];
        x?: number;
        y?: number;
        targetElement?: HTMLElement;
        originalRefBlockIDs?: IObject;
        isBacklink: boolean;
    }) => void;
    updateProtyleToolbar(toolbar: Array<string | IMenuItem>): (string | IMenuItem)[];
    set protyleOptions(options: IProtyleOptions);
    get protyleOptions(): IProtyleOptions;
}
