/// <reference types="./src/types" />
import { Tab } from "../Tab";
import { Model } from "../Model";
import { Tree } from "../../util/Tree";
import { App } from "../../index";
export declare class Outline extends Model {
    tree: Tree;
    element: HTMLElement;
    headerElement: HTMLElement;
    type: "pin" | "local";
    blockId: string;
    isPreview: boolean;
    private preFilterExpandIds;
    constructor(options: {
        app: App;
        tab: Tab;
        blockId: string;
        type: "pin" | "local";
        isPreview: boolean;
    });
    private bindSort;
    updateDocTitle(ial?: IObject, count?: number): void;
    private onTransaction;
    setCurrent(nodeElement: HTMLElement): void;
    setCurrentByPreview(nodeElement: Element): void;
    private setCurrentById;
    update(data: IWebSocketData, callbackId?: string): void;
    saveExpendIds(): void;
    /**
     * 应用大纲筛选
     */
    private setFilter;
    /**
     * 获取标题元素的实际标题级别（H1=1, H2=2, 等等）
     * @param element li元素
     * @returns 标题级别（1-6）
     */
    private getHeadingLevel;
    /**
     * 展开到指定标题级别
     * @param targetLevel 目标标题级别，1-6级（H1-H6），6级表示全部展开
     */
    private expandToLevel;
    /**
     * 显示展开层级菜单
     */
    private showExpandLevelMenu;
    /**
     * 切换同层级的所有标题的展开/折叠状态（基于标题级别而不是DOM层级）
     */
    private collapseSameLevel;
    private collapseChildren;
    /**
     * 显示右键菜单
     */
    private showContextMenu;
    private getProtyleAndBlockElement;
    /**
     * 生成标题级别转换菜单项
     */
    private genHeadingTransform;
}
