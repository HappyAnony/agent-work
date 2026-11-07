/// <reference types="./src/types" />
export declare class EventBus<DetailType = any> {
    private eventTarget;
    constructor(name?: string);
    on(type: TEventBus, listener: (event: CustomEvent<DetailType>) => void): void;
    once(type: TEventBus, listener: (event: CustomEvent<DetailType>) => void): void;
    off(type: TEventBus, listener: (event: CustomEvent<DetailType>) => void): void;
    emit(type: TEventBus, detail?: DetailType): boolean;
}
export declare const emitOpenMenu: (options: {
    plugins: import("./index").Plugin[];
    type: TEventBus;
    detail: any;
    separatorPosition?: "top" | "bottom";
}) => void;
