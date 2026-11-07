import type {App} from "../../index";
/// #if !BROWSER
import {ipcRenderer} from "electron";
import {Constants} from "../../constants";
/// #endif

export const sendGlobalShortcut = (app: App) => {
    /// #if !BROWSER
    const hotkeys = [window.siyuan.config.keymap.general.toggleWin.custom];
    app.plugins.forEach(plugin => {
        plugin.commands.forEach(command => {
            if (command.globalCallback) {
                hotkeys.push(command.customHotkey);
            }
        });
    });
    ipcRenderer.send(Constants.WIKI_HOTKEY, {
        languages: window.siyuan.languages["_trayMenu"],
        hotkeys
    });
    /// #endif
};


export const sendUnregisterGlobalShortcut = (app: App) => {
    /// #if !BROWSER
    ipcRenderer.send(Constants.WIKI_CMD, {
        cmd: "unregisterGlobalShortcut",
        accelerator: window.siyuan.config.keymap.general.toggleWin.custom
    });
    app.plugins.forEach(plugin => {
        plugin.commands.forEach(command => {
            if (command.globalCallback) {
                ipcRenderer.send(Constants.WIKI_CMD, {
                    cmd: "unregisterGlobalShortcut",
                    accelerator: command.customHotkey
                });
            }
        });
    });
    /// #endif
};
