/// <reference types="./src/types" />
export declare class Title {
    element: HTMLElement;
    editElement: HTMLElement;
    private timeout;
    constructor(protyle: IProtyle);
    private rename;
    setTitle(title: string): void;
    render(protyle: IProtyle, response: IWebSocketData): boolean;
}
