import {Constants} from "../constants";

let preferences: Config.IKeymap;

function mergeKeys(target: Record<string, Config.IKey>, source?: Record<string, Config.IKey>) {
    for (const [name, key] of Object.entries(target)) {
        const saved = source?.[name];
        if (typeof saved?.custom === "string") {
            key.custom = saved.custom;
        }
    }
}

export function loadPortableKeymap(saved?: Config.IKeymap) {
    preferences = defaultPortableKeymap();
    mergeKeys(preferences.general, saved?.general);
    for (const section of Object.keys(preferences.editor) as (keyof Config.IKeymapEditor)[]) {
        mergeKeys(preferences.editor[section], saved?.editor?.[section]);
    }
    if (saved?.plugin) {
        preferences.plugin = structuredClone(saved.plugin);
    }
}

export function defaultPortableKeymap() {
    const keys = structuredClone(Constants.WIKI_KEYMAP);
    keys.editor.general.redo = {custom: "⇧⌘Z", default: "⇧⌘Z"};
    return keys;
}

export function portableKeymapPreferences() {
    return structuredClone(preferences ?? defaultPortableKeymap());
}

export function activePortableKeymap(): Config.IKeymap {
    const active = structuredClone(Constants.WIKI_KEYMAP);
    for (const key of Object.values(active.general)) {
        key.custom = "";
    }
    const saved = preferences ?? defaultPortableKeymap();
    const supported = {
        general: new Set(["undo", "redo"]),
        insert: new Set(["bold", "italic", "underline", "strike", "mark", "sup", "sub", "kbd", "inline-code", "link", "appearance", "lastUsed", "inline-math", "memo"]),
    };
    for (const section of Object.keys(active.editor) as (keyof Config.IKeymapEditor)[]) {
        for (const [name, key] of Object.entries(active.editor[section])) {
            key.custom = (section === "general" || section === "insert") && supported[section].has(name)
                ? saved.editor[section][name].custom
                : "";
        }
    }
    active.plugin = {};
    return active;
}
