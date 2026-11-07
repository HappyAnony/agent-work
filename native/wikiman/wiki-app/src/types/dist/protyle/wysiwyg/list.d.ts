export declare const updateListOrder: (listElement: Element, sIndex?: number) => void;
export declare const genListItemElement: (listItemElement: Element, offset?: number, wbr?: boolean) => HTMLElement;
export declare const addSubList: (protyle: IProtyle, nodeElement: Element, range: Range) => void;
export declare const listIndent: (protyle: IProtyle, liItemElements: Element[], range: Range) => void;
export declare const breakList: (protyle: IProtyle, blockElement: Element, range: Range) => void;
/**
 * 缩进列表
 * @param protyle
 * @param liItemElements
 * @param range
 * @param isDelete
 * @param deleteElement 末尾反向删除时才会传入
 */
export declare const listOutdent: (protyle: IProtyle, liItemElements: Element[], range: Range, isDelete?: boolean, deleteElement?: Element) => void;
