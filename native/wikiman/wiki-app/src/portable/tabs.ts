import type {App} from "../index";
import {Tab} from "../layout/Tab";
import {MenuItem} from "../menus/Menu";
import {newFile} from "../util/newFile";
import {escapeHtml} from "../util/escape";
import {matchHotKey} from "../protyle/util/hotKey";
import {portableKeymapPreferences} from "./keymap";
import type {FrameTabs, TabCommand, TabIntent, TabPresentation} from "./bridge";

const transferType = "application/x-wiki-scoped-tab";
const tabID = /^[a-f0-9-]{36}$/;

export function mountScopedTabs(
    workspace: HTMLElement, pane: HTMLElement, app: App, notice: (message: string) => void,
    currentDocument?: () => {box: string; path: string},
): FrameTabs & {setTitle(title: string): void; destroy(): void} {
    const header = document.createElement("div");
    header.className = "fn__flex fn__none";
    header.dataset.scopedTabs = "";
    header.innerHTML = `<ul class="fn__flex layout-tab-bar" role="tablist"></ul>
<ul class="layout-tab-bar layout-tab-bar--readonly fn__flex-1"><li class="item item--readonly">
<button type="button" data-type="new" class="block__icon block__icon--show"><svg><use xlink:href="#iconAdd"></use></svg></button>
<span class="fn__flex-1"></span>
<button type="button" data-type="more" class="block__icon block__icon--show"><svg><use xlink:href="#iconDown"></use></svg></button>
</li></ul>`;
    pane.prepend(header);
    const bar = header.querySelector<HTMLElement>('[role="tablist"]');
    const languages = window.siyuan.languages;
    const newButton = header.querySelector<HTMLButtonElement>('[data-type="new"]');
    const moreButton = header.querySelector<HTMLButtonElement>('[data-type="more"]');
    newButton.setAttribute("aria-label", languages.newFile);
    moreButton.setAttribute("aria-label", languages.switchTab);
    newButton.addEventListener("click", () => {
        const current = currentDocument?.();
        const notebook = current?.box || window.siyuan.notebooks?.find(item => !item.closed)?.id;
        if (!notebook) { notice(languages.newFileTip); return; }
        newFile({app, notebookId:notebook, currentPath:current?.path || "/", useSavePath:true});
    });
    let presentation: TabPresentation;
    let send: ((intent: TabIntent) => void) | undefined;
    let title = "";
    let sendTitle: ((title: string) => void) | undefined;
    let dragTab = "";
    const elements = new Map<string,Tab>();
    const preview = document.createElement("div");
    preview.className = "layout-tab-container__drag fn__none";
    preview.style.cssText = "position:absolute;pointer-events:none;z-index:9;background:var(--b3-theme-primary-lightest)";
    pane.style.position = "relative";
    pane.append(preview);

    const act = (command: TabCommand, tabId: string, targetId?: string) => {
        window.siyuan.menus.menu.remove();
        send?.({command,tabId,targetId});
    };
    const menu = (id:string, event:MouseEvent) => {
        const value=presentation.tabs.find(tab=>tab.id===id);
        if(!value){return;}
        const common=window.siyuan.menus.menu;
        common.remove();
        const options: [TabCommand,string,boolean?][] = [
            ["pin",value.pinned?languages.unpin:languages.pin],
            ["close",languages.closeTab],
            ["close-others",languages.closeOthers],
            ["close-all",languages.closeAll],
            ["close-left",languages.closeLeft],
            ["close-right",languages.closeRight],
            ["split-copy-right",languages.splitLR],
            ["split-copy-down",languages.splitTB],
            ["split-right",languages.splitMoveR,presentation.tabs.length<2],
            ["split-down",languages.splitMoveB,presentation.tabs.length<2],
        ];
        for(const [command,label,disabled] of options){
            common.append(new MenuItem({id:command,label,disabled,click:()=>act(command,id)}).element);
        }
        common.popup({x:event.clientX,y:event.clientY});
    };
    moreButton.addEventListener("click",()=>{
        const common=window.siyuan.menus.menu;
        common.remove();
        for(const tab of presentation.tabs){
            common.append(new MenuItem({label:escapeHtml(tab.title),current:tab.id===presentation.active,action:"iconCloseRound",
                click:(_,event)=>act((event.target as Element).closest(".b3-menu__action")?"close":"select",tab.id)}).element);
        }
        const rect=moreButton.getBoundingClientRect();
        common.popup({x:rect.left,y:rect.bottom});
    });
    const closeTarget = (event: Event) => (event.target as Element).closest(".item__close");
    const build = (id:string,title:string) => {
        const tab = new Tab({title,icon:"iconFile"},true);
        const element = tab.headElement;
        element.dataset.scopedTab = id;
        element.setAttribute("role","tab");
        element.addEventListener("click",event=>act(closeTarget(event)?"close":"select",id));
        element.addEventListener("auxclick",event=>{if(event.button===1){event.preventDefault();act("close",id);}});
        element.addEventListener("contextmenu",event=>{event.preventDefault();menu(id,event);});
        element.addEventListener("keydown",event=>{
            if(event.key==="Enter"||event.key===" "){event.preventDefault();act("select",id);}
            else if(event.key==="Delete"){event.preventDefault();act("close",id);}
            else if(event.key==="ArrowLeft"||event.key==="ArrowRight"){
                event.preventDefault();
                const index=presentation.tabs.findIndex(tab=>tab.id===id);
                const next=presentation.tabs[index+(event.key==="ArrowRight"?1:-1)];
                if(next){act("select",next.id);}
            }
        });
        element.addEventListener("dragstart",event=>{
            dragTab=id;
            event.dataTransfer?.setData(transferType,id);
            if(event.dataTransfer){event.dataTransfer.effectAllowed="move";}
        });
        element.addEventListener("dragend",()=>{dragTab="";preview.classList.add("fn__none");});
        return tab;
    };
    const dropEdge=(event:DragEvent):TabCommand|undefined=>{
        const rect=pane.getBoundingClientRect();
        if(header.contains(event.target as Node)){return;}
        const x=(event.clientX-rect.left)/rect.width,y=(event.clientY-rect.top)/rect.height;
        if(x<.2){return "split-left";}if(x>.8){return "split-right";}
        if(y<.2){return "split-up";}if(y>.8){return "split-down";}
    };
    const isTabDrag=(event:DragEvent)=>!!event.dataTransfer?.types.includes(transferType);
    const over=(event:DragEvent)=>{
        if(!isTabDrag(event)){return;}
        event.preventDefault();event.stopPropagation();
        const edge=dropEdge(event), headerTarget=(event.target as Element).closest("[data-scoped-tab]");
        preview.classList.toggle("fn__none",!edge);
        event.dataTransfer.dropEffect=edge||headerTarget?"move":"none";
        if(!edge){return;}
        const horizontal=edge==="split-left"||edge==="split-right";
        Object.assign(preview.style,{left:edge==="split-right"?"50%":"0",top:edge==="split-down"?"50%":"0",
            width:horizontal?"50%":"100%",height:horizontal?"100%":"50%"});
    };
    const drop=(event:DragEvent)=>{
        if(!isTabDrag(event)){return;}
        event.preventDefault();event.stopPropagation();preview.classList.add("fn__none");
        const id=event.dataTransfer.getData(transferType)||dragTab;
        if(!tabID.test(id)){return;}
        const target=(event.target as Element).closest<HTMLElement>("[data-scoped-tab]");
        if(target){
            const rect=target.getBoundingClientRect();
            act(event.clientX<rect.left+rect.width/2?"move-before":"move-after",id,target.dataset.scopedTab);
        }else{
            const edge=dropEdge(event);
            if(edge){act(edge,id);}
        }
    };
    const leave=(event:DragEvent)=>{if(!pane.contains(event.relatedTarget as Node)){preview.classList.add("fn__none");}};
    pane.addEventListener("dragover",over,true);pane.addEventListener("drop",drop,true);pane.addEventListener("dragleave",leave);
    const keys=(event:KeyboardEvent)=>{
        if(!presentation?.tabs.length || event.isComposing || (event.target as Element).closest("input,textarea,select,.b3-dialog--open")){return;}
        if(matchHotKey(portableKeymapPreferences().general.closeTab.custom,event)){
            event.preventDefault();event.stopPropagation();act("close",presentation.active);
        }
    };
    document.addEventListener("keydown",keys,true);
    return {
        onIntent(callback){send=callback;},
        onTitle(callback){sendTitle=callback;if(title){sendTitle(title);}},
        setTitle(next){if(next && title!==next){title=next;sendTitle?.(next);}},
        report:notice,
        present(value){
            presentation=value;
            workspace.classList.toggle("portable-desktop--secondary",!value.sidebar);
            for(const child of Array.from(workspace.children) as HTMLElement[]){
                if(child!==pane){
                    if(value.sidebar){child.style.removeProperty("display");}
                    else{child.style.setProperty("display","none","important");}
                }
            }
            pane.classList.toggle("layout__wnd--active",value.focused);
            header.classList.remove("fn__none");
            newButton.hidden=window.siyuan.config.readonly || window.siyuan.config.editor.readOnly;
            moreButton.hidden=value.tabs.length===0;
            for(const [id,tab] of elements){if(!value.tabs.some(item=>item.id===id)){tab.headElement.remove();elements.delete(id);}}
            value.tabs.forEach((item,index)=>{
                const tab=elements.get(item.id)||build(item.id,item.title);
                elements.set(item.id,tab);
                if(tab.title!==item.title){tab.updateTitle(item.title);}
                const element=tab.headElement, active=item.id===value.active;
                element.classList.toggle("item--focus",active);
                element.classList.toggle("item--pin",item.pinned);
                element.classList.toggle("item--unupdate",item.dirty);
                element.setAttribute("aria-selected",String(active));
                element.setAttribute("aria-label",item.title);
                element.tabIndex=active?0:-1;
                if(bar.children[index]!==element){bar.insertBefore(element,bar.children[index]??null);}
            });
        },
        destroy(){send=undefined;sendTitle=undefined;document.removeEventListener("keydown",keys,true);header.remove();preview.remove();},
    };
}
