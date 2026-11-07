import {EditorSession,SessionError} from "./session";
import {installEditorFrameBridge,type SavedTabs} from "./bridge";
import {mountPortableProtyle} from "./mount";
import {initializePortableUI, UIConfiguration} from "./ui";
import {markPortableWindow} from "./boundary";
import {mountEmptyPortableDesktop} from "./layout";
import {addScript} from "../protyle/util/addScript";

async function boot():Promise<void>{
    const params=new URLSearchParams(location.hash.slice(1));
    const parentOrigin=params.get("parentOrigin"),channelId=params.get("channelId");
    if(window.parent===window || !parentOrigin || !channelId || !/^[a-zA-Z0-9-]{16,128}$/.test(channelId)){throw new SessionError("invalid_parent_identity");}
    const origin=new URL(parentOrigin);
    if(!((origin.protocol==="http:" && ["127.0.0.1","localhost","[::1]"].includes(origin.hostname)) || origin.protocol==="tauri:" && origin.host==="localhost") || parentOrigin!==(origin.protocol==="tauri:"?"tauri://localhost":origin.origin)){throw new SessionError("invalid_parent_identity");}
    // Only public channel information is present in the fragment. Capabilities
    // arrive once, on a verified private port, never in browser persistence.
    markPortableWindow();
    installEditorFrameBridge({parentWindow:window.parent,parentOrigin,channelId,mount:async(payload,dirty,signal)=>{
        if(payload.baseUrl!==location.origin){throw new SessionError("invalid_server_origin");}
        const session=new EditorSession(payload.baseUrl,payload.view,{dirty:(_view,value)=>dirty(value)});
        const [bootstrap, local] = await Promise.all([
            session.requestUI("/api/system/getConf") as Promise<{conf?: UIConfiguration}>,
            (session.requestUI("/api/storage/getLocalStorage") as Promise<Record<string, unknown>>).catch(error => {
                // Preferences are derived local state. An unreadable cache
                // cannot block entry to an otherwise valid source document.
                if (error instanceof SessionError && ["operation_failed", "not_found"].includes(error.code)) {
                    return {};
                }
                throw error;
            }),
        ]);
        const preferences = bootstrap?.conf;
        const hostLanguages: Record<string,string> = {en:"en_US",zh:"zh_CN",ja:"ja_JP",es:"es_ES",fr:"fr_FR",de:"de_DE",ru:"ru_RU",pt:"pt_PT"};
        if (payload.locale && preferences?.appearance) {
            preferences.appearance.lang = hostLanguages[payload.locale] ?? "en_US";
        }
        const language = preferences?.appearance?.lang;
        if (!language || !/^[a-z]{2}_[A-Z]{2,3}$/.test(language)) { throw new SessionError("invalid_ui_configuration"); }
        const response = await fetch("/appearance/langs/" + language + ".json",{credentials:"omit",cache:"no-store",redirect:"error",signal});
        if(!response.ok){throw new SessionError("editor_language_unavailable");}
        const languages = await response.json();
        if (signal.aborted) { throw new SessionError("stale_mount"); }
        initializePortableUI(languages, preferences, local);
        // The packaged catalogue contains Unicode entries and never needs a
        // device or knowledge-repository path.
        void fetch("/appearance/emojis/conf.json", {credentials:"omit", redirect:"error", signal})
            .then(result => result.ok ? result.json() : [])
            .then((categories: IEmoji[]) => {
                if (!signal.aborted && Array.isArray(categories)) { window.siyuan.emojis.push(...categories); }
            }).catch(() => undefined);
        const icon = preferences.appearance.icon === "ant" ? "ant" : "material";
        await addScript(`/appearance/icons/${icon}/icon.js`, "portableBuiltinIcons");
        if (signal.aborted) { throw new SessionError("stale_mount"); }
        const element = document.getElementById("editor");
        const app = {appId:"wiki-portable",plugins:[]} as any;
        const editor = payload.document
            ? (await mountPortableProtyle({element, session, document:payload.document, app, hostPluginLayout: payload.hostPluginLayout})).binding
            : await mountEmptyPortableDesktop(element, session, app);
        document.body.dataset.editorReady="true";
        if (session.view.mode !== "wikiDesktop") { return editor; }
        return Object.assign(editor,{
            savedTabs:(local as Record<string,unknown>)["local-desktop-tabs"] as SavedTabs | undefined,
            saveTabs:async(value:SavedTabs)=>{
                await session.requestUI("/api/storage/setLocalStorageVal",{key:"local-desktop-tabs",val:value});
            },
        });
    }});
    window.parent.postMessage({type:"wiki:editor:booted",channelId},parentOrigin);
}
void boot().catch(error=>{const message=document.createElement("p");message.setAttribute("role","alert");message.textContent=error instanceof Error?error.message:"editor_boot_failed";document.body.appendChild(message);});
