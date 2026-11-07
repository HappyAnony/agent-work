export declare const removeBlock: (protyle: IProtyle, blockElement: Element, range: Range, type: "Delete" | "Backspace" | "remove") => Promise<void>;
export declare const moveToPrevious: (blockElement: Element, range: Range, isDelete: boolean) => Range;
export declare const removeImage: (imgSelectElement: Element, nodeElement: HTMLElement, range: Range, protyle: IProtyle) => void;
