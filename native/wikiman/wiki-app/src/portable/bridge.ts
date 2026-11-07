import {validEditorParentOrigin} from "./parentOrigin";
import {DocumentKey, EditorView, SessionError} from "./session";
import {ProtyleBinding} from "./protyle";

export interface FrameMount {
    baseUrl: string;
    streamBaseUrl?: string;
    view: EditorView;
    document: DocumentKey | null;
    focus?: {blockId:string;hash:string};
    hostPluginLayout?: boolean;
    locale?: string;
}
export interface PluginHostAction {id: string; label: string; region: "left" | "right" | "bottom" | "status"}
export type TabCommand = "select" | "close" | "pin" | "close-others" | "close-all" | "close-left" | "close-right" |
    "split-left" | "split-right" | "split-up" | "split-down" | "split-copy-right" | "split-copy-down" | "move-before" | "move-after";
export interface TabIntent {command: TabCommand; tabId: string; targetId?: string}
export interface TabPresentation {
    tabs: {id: string; title: string; pinned: boolean; dirty: boolean}[];
    active: string;
    sidebar: boolean;
    focused: boolean;
}
export type SavedTabTree = {tabs: (DocumentKey & {pinned:boolean})[]; active:number} |
    {direction:"horizontal"|"vertical"; ratio:number; children:[SavedTabTree,SavedTabTree]};
export interface SavedTabs {version:1; root:SavedTabTree|null; focused:number}
export interface FrameTabs {
    present(value: TabPresentation): void;
    onIntent(send: (intent: TabIntent) => void): void;
    onTitle(send: (title: string) => void): void;
    report(error: string): void;
}
const tabCommands = new Set(["select","close","pin","close-others","close-all","close-left","close-right",
    "split-left","split-right","split-up","split-down","split-copy-right","split-copy-down","move-before","move-after"]);
const tabID = /^[a-f0-9-]{36}$/;
type FrameEditor = Pick<ProtyleBinding, "detach" | "onDetach" | "setReferenceNavigator" | "prepareRelease" |
    "focusReference" | "canRestoreHistory" | "handleFocusedHistory" | "prepareTabSwitch" | "focusTab"> & {
        session?: Pick<ProtyleBinding["session"], "onWorkspaceEvent">; tabs?: FrameTabs; savedTabs?: SavedTabs; saveTabs?(value:SavedTabs):Promise<void>;
        pluginSystemShortcuts?: string[]; reportPluginSystemShortcuts?: (chords: string[]) => void; runPluginSystemShortcut?: (chord: string) => void;
        pluginHostActions?: PluginHostAction[]; reportPluginHostActions?: (actions: PluginHostAction[]) => void;
        runPluginHostAction?: (id: string) => void;
    };
type FrameRequest = "flush" | "dispose" | "focus" | "history" | "switch" | "activate" | "saveTabs";
const responseType = (request: unknown) => request === "history" ? "historyResult" : request === "focus" ? "focusResult" :
    request === "switch" || request === "activate" || request === "saveTabs" ? "tabResult" : "result";
const drainsInput = (request: unknown) => request === "flush" || request === "dispose";
interface Identity {channelId: string; storeId: string; pageId: string; sessionId: string}
function identity(channelId: string, view: EditorView): Identity {return {channelId,storeId:view.storeId,pageId:view.pageId,sessionId:view.sessionId};}
function matches(value: Record<string, unknown>, expected: Identity): boolean {return Object.entries(expected).every(([key,item])=>value[key]===item);}

/** The embedding host provides its exact origin/window and a fresh public
 * challenge. No token enters a URL, wildcard targetOrigin or global storage.
 * This installs only a handshake; it does not silently boot a legacy editor. */
export function installEditorFrameBridge(options: {
    parentWindow: Window;
    parentOrigin: string;
    channelId: string;
    mount(payload: FrameMount, reportDirty: (dirty: boolean) => void, signal: AbortSignal): Promise<FrameEditor>;
    timeoutMs?: number;
}): () => void {
    if (!options.channelId || !validEditorParentOrigin(options.parentOrigin)) {throw new SessionError("invalid_parent_identity");}
    let port: MessagePort | null=null, binding: FrameEditor | null=null, used=false, disposed=false;
    const lifetime = new AbortController();
    const stop=()=>{disposed=true;lifetime.abort();window.removeEventListener("message",receive);port?.close();binding?.detach();};
    const receive=(event:MessageEvent)=>{
        if (disposed || used || event.source!==options.parentWindow || event.origin!==options.parentOrigin || event.ports.length!==1) {return;}
        const data=event.data;
        if (!data || data.type!=="wiki:editor:init" || data.channelId!==options.channelId || !data.payload?.view ||
            (data.payload.document === null ? data.payload.view.mode !== "wikiDesktop" : !data.payload.document)) {return;}
        used=true; port=event.ports[0];
        const activePort=port, payload=data.payload as FrameMount;
        const expected=identity(options.channelId,payload.view);
        const send=(value:Record<string,unknown>)=>{if(!disposed){activePort.postMessage({...value,...expected});}};
        const reportDirty=(dirty:boolean)=>send({type:"dirty",dirty});
        const navigation=new Map<string,{resolve():void;reject(error:unknown):void;timer:ReturnType<typeof setTimeout>}>();
        const cancelNavigation=()=>{for(const wait of navigation.values()){clearTimeout(wait.timer);wait.reject(new SessionError("editor_channel_closed"));}navigation.clear();};
        // A timed-out mount is detached even if its async read completes later.
        let timer:ReturnType<typeof setTimeout>;
        const mounted=options.mount(payload,reportDirty,lifetime.signal).then(value=>{
            if(disposed){value.detach();throw new SessionError("stale_mount");}binding=value;
            value.onDetach(cancelNavigation);
            if (value.session) {
                let treeDigest: string | undefined;
                value.onDetach(value.session.onWorkspaceEvent(message => {
                    const digest = (message.event as {treeSourceDigest?: unknown} | undefined)?.treeSourceDigest;
                    if (message.type === "workspaceEvent" && typeof digest === "string" && /^[a-f0-9]{64}$/.test(digest) && digest !== treeDigest) {
                        treeDigest = digest;
                        send({type:"treeDigest",digest});
                    }
                }));
            }
            value.tabs?.onIntent(intent=>send({type:"tabIntent",intent}));
            value.tabs?.onTitle(title=>send({type:"tabTitle",title}));
            if (value.tabs) {
                const focused = () => send({type:"tabFocus"});
                document.addEventListener("focusin",focused);
                window.addEventListener("focus",focused);
                value.onDetach(() => {document.removeEventListener("focusin",focused);window.removeEventListener("focus",focused);});
            }
            value.setReferenceNavigator((blockId,reuseCurrent)=>new Promise<void>((resolve,reject)=>{
                if(disposed||navigation.size){reject(new SessionError("reference_navigation_busy"));return;}
                const requestId=crypto.randomUUID(),timer=setTimeout(()=>{navigation.delete(requestId);reject(new SessionError("reference_navigation_timeout"));},30000);
                navigation.set(requestId,{resolve,reject,timer});send({type:"navigate",requestId,blockId,document:payload.document,
                    ...(reuseCurrent?{reuseCurrent:true}:{})});
            }));
            if(payload.focus){value.focusReference(payload.focus.blockId,payload.focus.hash,false);}
            return value;
        });
        const ready=Promise.race([mounted,new Promise<never>((_,reject)=>{timer=setTimeout(()=>reject(new SessionError("editor_mount_timeout")),options.timeoutMs??15000);})]).finally(()=>clearTimeout(timer));
        void ready.then(value=>{
            send({type:"ready",savedTabs:value.savedTabs});
            value.reportPluginSystemShortcuts = chords => send({type:"pluginSystemShortcuts",chords});
            value.reportPluginSystemShortcuts(value.pluginSystemShortcuts ?? []);
            if (payload.hostPluginLayout) {
                value.reportPluginHostActions = actions => send({type:"pluginHostActions",actions});
                value.reportPluginHostActions(value.pluginHostActions ?? []);
            }
        }).catch(error=>{send({type:"error",code:error instanceof SessionError?error.code:"editor_mount_failed",dirty:true});stop();});
        let queue:Promise<void>=Promise.resolve(), lastCommand=0;
        activePort.onmessage=(message:MessageEvent)=>{
            const request=message.data as Record<string,unknown>;
            if(request&&matches(request,expected)&&request.type==="navigationResult"&&typeof request.requestId==="string"){
                const wait=navigation.get(request.requestId);if(!wait){return;}navigation.delete(request.requestId);clearTimeout(wait.timer);
                if(request.ok===true){wait.resolve();}else{wait.reject(new SessionError(String(request.code??"reference_navigation_failed")));}return;
            }
            if (!request || !matches(request,expected)) {return;}
            if (request.type === "pluginHostAction" && payload.hostPluginLayout && typeof request.id === "string") {
                binding?.runPluginHostAction?.(request.id);
                return;
            }
            if (request.type === "pluginSystemShortcut" && typeof request.chord === "string") {
                binding?.runPluginSystemShortcut?.(request.chord);
                return;
            }
            if (request.type === "tabs") {
                binding?.tabs?.present(request.value as TabPresentation);
                return;
            }
            if (request.type === "tabError" && typeof request.code === "string") { binding?.tabs?.report(request.code); return; }
            if (typeof request.requestId!=="string" || !["flush","dispose","focus","history","switch","activate","saveTabs"].includes(String(request.type))) {return;}
            const sequence = /^command-([1-9][0-9]{0,15})$/.exec(request.requestId);
            const value = sequence ? Number(sequence[1]) : 0;
            if (!Number.isSafeInteger(value) || value <= lastCommand) { return; }
            lastCommand = value;
            queue=queue.then(async()=>{
                try {
                    const current=await ready;
                    if (request.type === "history") {
                        if (disposed || typeof request.redo !== "boolean") { throw new SessionError("editor_history_unavailable"); }
                        const handled = current.handleFocusedHistory(request.redo);
                        send({type:"historyResult",requestId:request.requestId,ok:true,handled});return;
                    }
                    if(request.type==="focus"){
                        const focus = (request.focus ?? payload.focus) as FrameMount["focus"];
                        if(!focus || !/^\d{14}-[a-z0-9]{7}$/.test(focus.blockId) || !/^[a-f0-9]{64}$/.test(focus.hash) ||
                            !current.canRestoreHistory){throw new SessionError("reference_focus_unavailable");}
                        current.focusReference(focus.blockId,focus.hash);
                        send({type:"focusResult",requestId:request.requestId,ok:true});return;
                    }
                    if (request.type === "switch" || request.type === "activate") {
                        if (request.type === "switch") { await current.prepareTabSwitch(); }
                        else { current.focusTab(); }
                        send({type:"tabResult",requestId:request.requestId,ok:true});return;
                    }
                    if (request.type === "saveTabs") {
                        if (!current.saveTabs) { throw new SessionError("editor_layout_unavailable"); }
                        await current.saveTabs(request.layout as SavedTabs);
                        send({type:"tabResult",requestId:request.requestId,ok:true});return;
                    }
                    await current.prepareRelease();
                    send({type:"result",requestId:request.requestId,ok:true,dirty:false});
                    if(request.type==="dispose"){stop();}
                } catch(error){send({type:responseType(request.type),requestId:request.requestId,ok:false,...(drainsInput(request.type)?{dirty:true}:{}),code:error instanceof SessionError?error.code:"editor_command_failed"});}
            });
        };
        activePort.start();
    };
    window.addEventListener("message",receive);
    return stop;
}

/** Host half for an already loaded controlled iframe. Every reply is bound to
 * the private channel and full source identity; no ambient window messages can
 * acknowledge a save/release. UI mounting/source switching stays host-owned. */
export function connectEditorFrame(options: {
    frameWindow: Window; frameOrigin: string; channelId: string; payload: FrameMount;
    dirty(dirty:boolean):void; timeoutMs?:number;
    navigate?(blockId:string,reuseCurrent?:boolean):Promise<void>;
    tabIntent?(intent:TabIntent):Promise<void>;
    tabTitle?(title:string):void;
    treeDigest?(digest:string):void;
    tabFocus?():void;
    savedTabs?(value:unknown):void;
    pluginSystemShortcuts?(chords: string[]):void;
    pluginHostActions?(actions: PluginHostAction[]):void;
}) {
    if(options.frameOrigin!==new URL(options.payload.baseUrl).origin){throw new SessionError("invalid_frame_origin");}
    const channel=new MessageChannel(), expected=identity(options.channelId,options.payload.view);
    const pending=new Map<string,{type:FrameRequest;resolve(handled:boolean):void;reject(error:unknown):void;timer:ReturnType<typeof setTimeout>}>();
    let closed=false, resolveReady!:()=>void,rejectReady!:(error:unknown)=>void;
    let mounted=false,navigating=false,commandSequence=0;const seenNavigation=new Set<string>();
    const ready=new Promise<void>((resolve,reject)=>{resolveReady=resolve;rejectReady=reject;});
    const readyTimer=setTimeout(()=>{rejectReady(new SessionError("editor_mount_timeout"));close();},options.timeoutMs??15000);
    const close=()=>{if(closed){return;}closed=true;clearTimeout(readyTimer);channel.port1.close();rejectReady(new SessionError("editor_channel_closed"));for(const wait of pending.values()){clearTimeout(wait.timer);wait.reject(new SessionError("editor_channel_closed"));}pending.clear();};
    channel.port1.onmessage=(message:MessageEvent)=>{
        const reply=message.data as Record<string,unknown>;if(!reply || !matches(reply,expected) || closed){return;}
        if(reply.type==="ready"){if(mounted){return;}mounted=true;options.savedTabs?.(reply.savedTabs);clearTimeout(readyTimer);resolveReady();return;}
        if (reply.type === "treeDigest" && mounted && typeof reply.digest === "string" && /^[a-f0-9]{64}$/.test(reply.digest)) {
            options.treeDigest?.(reply.digest);return;
        }
        if (reply.type === "tabFocus" && mounted) { options.tabFocus?.(); return; }
        if (reply.type === "pluginHostActions" && mounted && options.payload.hostPluginLayout &&
            options.payload.view.mode === "wikiDesktop" && Array.isArray(reply.actions) && reply.actions.length <= 96 &&
            reply.actions.every(action => action && typeof action.id === "string" && /^p[0-7]-[0-9]{1,2}$/.test(action.id) &&
                typeof action.label === "string" && action.label.length > 0 && action.label.length <= 96 &&
                ["left","right","bottom","status"].includes(action.region))) {
            options.pluginHostActions?.(reply.actions);
            return;
        }
        if (reply.type === "pluginSystemShortcuts" && mounted && options.payload.view.mode === "wikiDesktop" &&
            Array.isArray(reply.chords) && reply.chords.length <= 24 && reply.chords.every(chord =>
                typeof chord === "string" && /^⌥⇧⌘[A-Z0-9]$/.test(chord))) {
            options.pluginSystemShortcuts?.(reply.chords);
            return;
        }
        if (reply.type === "tabTitle" && options.payload.view.mode === "wikiDesktop" &&
            typeof reply.title === "string" && reply.title.length > 0 && reply.title.length <= 4096) {
            options.tabTitle?.(reply.title);return;
        }
        if (reply.type === "tabIntent" && mounted && options.payload.view.mode === "wikiDesktop" && options.tabIntent) {
            const intent = reply.intent as TabIntent;
            if (!intent || !tabCommands.has(intent.command) || typeof intent.tabId !== "string" || !tabID.test(intent.tabId) ||
                (intent.targetId !== undefined && (typeof intent.targetId !== "string" || !tabID.test(intent.targetId)))) {return;}
            void options.tabIntent(intent).catch(error=>{
                if (!closed) {channel.port1.postMessage({type:"tabError",code:error instanceof Error?error.message:"editor_command_failed",...expected});}
            });
            return;
        }
        if(reply.type==="navigate"){
            const doc=reply.document as DocumentKey|null|undefined,id=reply.requestId;
            const owner = options.payload.document;
            const sameDocument = owner === null ? doc === null : doc?.boxId === owner.boxId && doc.docId === owner.docId;
            if(!mounted||typeof id!=="string"||!id||id.length>128||seenNavigation.has(id)||seenNavigation.size>=1024||!sameDocument||typeof reply.blockId!=="string"||!/^\d{14}-[a-z0-9]{7}$/.test(reply.blockId)){return;}
            seenNavigation.add(id);
            const respond=(ok:boolean,code?:string)=>{if(!closed){channel.port1.postMessage({type:"navigationResult",requestId:id,ok,code,...expected});}};
            if(navigating||!options.navigate){respond(false,navigating?"reference_navigation_busy":"reference_navigation_unavailable");return;}
            navigating=true;
            void Promise.resolve().then(()=>reply.reuseCurrent===true&&options.payload.view.mode==="wikiDesktop"
                ?options.navigate!(reply.blockId as string,true):options.navigate!(reply.blockId as string))
                .then(()=>respond(true),error=>respond(false,error instanceof Error?error.message:"reference_navigation_failed")).finally(()=>{navigating=false;});return;
        }
        if(reply.type==="dirty" && typeof reply.dirty==="boolean"){options.dirty(reply.dirty);return;}
        if(reply.type==="error"){options.dirty(true);rejectReady(new SessionError(String(reply.code)));close();return;}
        if(!["result","focusResult","historyResult","tabResult"].includes(String(reply.type)) || typeof reply.requestId!=="string"){return;}
        const wait=pending.get(reply.requestId);if(!wait){return;}
        if(responseType(wait.type)!==reply.type){return;}
        pending.delete(reply.requestId);clearTimeout(wait.timer);
        if(reply.ok===true && (!drainsInput(wait.type)||reply.dirty===false)){wait.resolve(reply.handled===true);}else{if(drainsInput(wait.type)){options.dirty(true);}wait.reject(new SessionError(String(reply.code??"unsaved_editor_state")));}
    };
    channel.port1.start();
    options.frameWindow.postMessage({type:"wiki:editor:init",channelId:options.channelId,payload:options.payload},options.frameOrigin,[channel.port2]);
    const request=async(type:FrameRequest,data?:{redo?:boolean;focus?:FrameMount["focus"];layout?:SavedTabs})=>{
        await ready;if(closed){throw new SessionError("editor_channel_closed");}
        const requestId="command-"+(++commandSequence);
        return new Promise<boolean>((resolve,reject)=>{
            const timer=setTimeout(()=>{pending.delete(requestId);if(drainsInput(type)){options.dirty(true);}reject(new SessionError(type==="focus"?"reference_focus_timeout":type==="history"?"editor_history_timeout":"editor_flush_timeout"));},options.timeoutMs??15000);
            pending.set(requestId,{type,resolve,reject,timer});channel.port1.postMessage({type,requestId,...data,...expected});
        });
    };
    return {
        pluginHostAction(id: string) {
            if (mounted && !closed) { channel.port1.postMessage({type:"pluginHostAction",id,...expected}); }
        },
        pluginSystemShortcut(chord: string) {
            if (mounted && !closed) { channel.port1.postMessage({type:"pluginSystemShortcut",chord,...expected}); }
        },
        ready, flush:async()=>{await request("flush");}, dispose:async()=>{await request("dispose");close();},
        focus:async(focus?:FrameMount["focus"])=>{await request("focus", {focus});},
        history:(redo:boolean)=>request("history",{redo}), close,
        prepareTabSwitch:async()=>{await request("switch");},
        activateTab:async()=>{await request("activate");},
        saveTabs:async(layout:SavedTabs)=>{await request("saveTabs",{layout});},
        presentTabs:(value:TabPresentation)=>{if (mounted && !closed) {channel.port1.postMessage({type:"tabs",value,...expected});}},
    };
}
