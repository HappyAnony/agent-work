/// <reference types="./src/types" />
interface IOperations {
    doOperations: IOperation[];
    undoOperations: IOperation[];
}
export declare class Undo {
    private hasUndo;
    redoStack: IOperations[];
    undoStack: IOperations[];
    constructor();
    undo(protyle: IProtyle): void;
    redo(protyle: IProtyle): void;
    private render;
    replace(doOperations: IOperation[], protyle: IProtyle): void;
    add(doOperations: IOperation[], undoOperations: IOperation[], protyle: IProtyle): void;
    clear(): void;
}
export declare const electronUndo: (event: KeyboardEvent) => boolean;
export {};
