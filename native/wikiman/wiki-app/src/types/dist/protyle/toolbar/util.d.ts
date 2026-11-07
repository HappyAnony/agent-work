export declare const previewTemplate: (pathString: string, element: Element, parentId: string) => void;
export declare const removeSearchMark: (element: HTMLElement) => void;
export declare const removeInlineType: (inlineElement: HTMLElement, type: string, range?: Range) => void;
export declare const toolbarKeyToMenu: (toolbar: Array<string | IMenuItem>) => IMenuItem[];
export declare const copyTextByType: (ids: string[], type: "ref" | "blockEmbed" | "protocol" | "protocolMd" | "hPath" | "id" | "webURL") => Promise<void>;
