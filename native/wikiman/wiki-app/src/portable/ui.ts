import {Constants} from "../constants";
import {Menu} from "../menus/Menu";
import {initMessage} from "../dialog/message";
import {activePortableKeymap, loadPortableKeymap} from "./keymap";

export interface UIConfiguration {
    appearance: {lang: Config.TLang; mode: number; modeOS?: boolean; themeLight?: string; themeDark?: string; icon?: string};
    langs?: {label: string; name: string}[];
    editor: Record<string, unknown> & {markdown?: Record<string, boolean>};
    fileTree?: IObject;
    search?: Config.ISearch;
    tag?: Config.ITag;
    graph?: Config.IGraph;
    flashcard?: Config.IFlashCard;
    readonly?: boolean;
    keymap?: Config.IKeymap;
    uiLayout?: Record<string, unknown>;
    system?: IObject;
}

// The frame owns a detached projection of the current page's UI preferences.
export function initializePortableUI(languages:IObject, preferences?:UIConfiguration, local?:Record<string, unknown>):void {
    loadPortableKeymap(preferences?.keymap);
    const editor={fontSize:16,fontFamily:"",fullWidth:true,spellcheck:false,displayNetImgMark:false,readOnly:false,
        allowHTMLBLockScript:false,fontSizeScrollZoom:false,codeLineWrap:false,codeTabSpaces:4,codeSyntaxHighlightLineNum:false,
        codeLigatures:false,displayBookmarkIcon:true,emoji:[] as string[],virtualBlockRef:false,floatWindowMode:2,dynamicLoadBlocks:192,
        blockRefDynamicAnchorTextMaxLen:96,listLogicalOutdent:false,listItemDotNumberClickFocus:false,rtl:false,justify:false,
        markdown:{inlineAsterisk:true,inlineUnderscore:true,inlineSup:true,inlineSub:true,inlineTag:true,inlineMath:true,inlineStrikethrough:true,inlineMark:true},
    };
    const configuredEditor = preferences?.editor;
    if (configuredEditor) {
        Object.assign(editor, configuredEditor, {
            allowHTMLBLockScript: false,
            markdown: {...editor.markdown, ...configuredEditor.markdown},
        });
    }
    const storage:Record<string,unknown>={};
    storage[Constants.LOCAL_FILESPATHS]=[];
    storage[Constants.LOCAL_HISTORY]={notebookId:"%",type:0,operation:"all",sideWidth:"256px",sideDocWidth:"256px",sideDiffWidth:"256px"};
    storage[Constants.LOCAL_IMAGES]={file:"1f4c4",note:"1f5c3",folder:"1f4d1"};
    storage[Constants.LOCAL_OUTLINE]={};
    storage[Constants.LOCAL_FILEPOSITION]={};storage[Constants.LOCAL_DIALOGPOSITION]={};storage[Constants.LOCAL_CODELANG]="plaintext";
    storage[Constants.LOCAL_EMOJIS]={currentTab:"emoji"};storage[Constants.LOCAL_FONTSTYLES]=[];
    storage[Constants.LOCAL_PDFTHEME]={light:"light",dark:"dark",annoColor:"var(--b3-pdf-background1)"};
    for (const key of [Constants.LOCAL_CODELANG, Constants.LOCAL_OUTLINE, Constants.LOCAL_FILEPOSITION, Constants.LOCAL_DIALOGPOSITION,
        Constants.LOCAL_FILESPATHS, Constants.LOCAL_HISTORY, Constants.LOCAL_FONTSTYLES, "local-singleoutlinecollapsed"]) {
        if (local && Object.prototype.hasOwnProperty.call(local, key)) { storage[key] = local[key]; }
    }
    (window as any).siyuan={
        zIndex:10,transactions:[],reqIds:{},backStack:[],layout:{},dialogs:[],blockPanels:[],closedTabs:[],
        ctrlIsPressed:false,altIsPressed:false,shiftIsPressed:false,isPublish:false,notebooks:[],
        config:{editor,lang:preferences?.appearance.lang??"en_US",langs:preferences?.langs??[],appearance:{lang:"en_US",mode:0,...preferences?.appearance},export:{paragraphBeginningSpace:false},
            // Legacy focus/unfocus commands fetch a workspace document. They
            // are not the controlled host reference/child-document router.
            keymap:activePortableKeymap(),
            uiLayout:preferences?.uiLayout??{},
            search:preferences?.search,tag:preferences?.tag,graph:preferences?.graph,flashcard:{deck:false,...preferences?.flashcard},
            fileTree:preferences?.fileTree??{},system:{container:"browser",...preferences?.system},account:{},sync:{enabled:false},readonly:preferences?.readonly??false},
        storage,languages,emojis:[{items:[]}],menus:{},
    };
    applyPortableAppearance(window.siyuan.config.appearance);
    for(const [id,cls] of [["commonMenu","b3-menu fn__none"],["tooltip","tooltip fn__none"],["message","b3-snackbars"]]){
        const node=document.createElement("div");node.id=id;node.className=cls;document.body.appendChild(node);
    }
    document.getElementById("commonMenu").innerHTML='<div class="b3-menu__title fn__none"><span class="b3-menu__label"></span></div><div class="b3-menu__items"></div>';
    window.siyuan.menus.menu=new Menu();
    initMessage();
}

export function applyPortableAppearance(appearance: UIConfiguration["appearance"]):void {
    const mode = appearance.modeOS ? (window.matchMedia("(prefers-color-scheme: dark)").matches ? 1 : 0) : appearance.mode;
    Object.assign(window.siyuan.config.appearance, appearance, {mode});
    window.siyuan.config.lang = appearance.lang;
    document.documentElement.lang = appearance.lang.replace("_", "-");
    document.documentElement.dataset.themeMode = mode === 1 ? "dark" : "light";
    document.documentElement.dataset.lightTheme = appearance.themeLight || "daylight";
    document.documentElement.dataset.darkTheme = appearance.themeDark || "midnight";
    const theme = document.querySelector<HTMLLinkElement>('link[href*="/appearance/themes/"]:not(#themeStyle)');
    if (!theme) { return; }
    const fallback = mode === 1 ? "midnight" : "daylight";
    theme.href = "/appearance/themes/" + fallback + "/theme.css";
    const selected = mode === 1 ? appearance.themeDark : appearance.themeLight;
    const custom = document.querySelector<HTMLLinkElement>("#themeStyle");
    if (selected && selected !== fallback && /^[A-Za-z0-9][A-Za-z0-9 _.-]{0,95}$/.test(selected)) {
        const link = custom ?? document.createElement("link");
        link.id = "themeStyle";
        link.rel = "stylesheet";
        link.href = "/appearance/themes/" + encodeURIComponent(selected) + "/theme.css";
        if (!custom) { theme.after(link); }
    } else {
        custom?.remove();
    }
}
