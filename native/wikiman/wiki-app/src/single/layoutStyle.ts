export function ensureSingleLayoutStyleElement(): void {
    if (document.getElementById("singleLayoutStyle")) {
        return;
    }
    const styleElement = document.createElement("style");
    styleElement.id = "singleLayoutStyle";
    styleElement.textContent = `
html,
body,
#singleWorkspace {
    height: 100%;
    margin: 0;
    overflow: hidden;
}

#singleWorkspace {
    display: flex;
    min-width: 0;
    background: var(--b3-theme-background);
    color: var(--b3-theme-on-background);
}

#singleEditorPane {
    min-width: 0;
    flex: 1 1 auto;
    height: 100%;
    overflow-y: auto;
}

#editor {
    height: 100%;
}

#singleOutline {
    width: 260px;
    min-width: 260px;
    max-width: 260px;
    flex-shrink: 0;
    height: 100%;
    border-right: 1px solid var(--b3-theme-surface-lighter);
    background: var(--b3-theme-surface);
    transition: width 0.2s ease, min-width 0.2s ease, max-width 0.2s ease;
}

#singleOutline.single-outline--collapsed {
    width: 36px;
    min-width: 36px;
    max-width: 36px;
    border-right: none;
    background: transparent;
}

#singleOutline.single-outline--collapsed .block__icons > *:not([data-type="togglePanel"]) {
    display: none;
}

#singleOutline.single-outline--collapsed .block__icons {
    justify-content: flex-start;
    padding: 0 4px;
}

#singleOutline.single-outline--collapsed > .fn__flex-1 {
    display: none;
}

#singleOutline .b3-list-item__graphic[style*="height: 22px"] {
    display: none;
}

#singleOutline .block__icons {
    flex-shrink: 0;
    padding: 0 4px;
    min-height: 36px;
}

#singleOutline .block__icons .block__icon {
    padding: 4px;
}

#singleOutline .block__icons .fn__space {
    width: 4px;
}

#singleOutline .block__icon[data-type="togglePanel"] {
    opacity: 1;
    min-width: 28px;
    width: 28px;
    font-size: 14px;
    text-align: center;
    font-weight: bold;
}

#singleOutline > .fn__flex-1 {
    min-height: 0;
    overflow: auto;
}

#singleOutline.single-outline--collapsed {
    width: 36px;
    min-width: 36px;
    max-width: 36px;
}

#singleOutline.single-outline--collapsed .block__logo,
#singleOutline.single-outline--collapsed .block__icons > .fn__flex-1,
#singleOutline.single-outline--collapsed .block__icons > .fn__space,
#singleOutline.single-outline--collapsed .block__icons > .search__label,
#singleOutline.single-outline--collapsed .block__icons > .block__icon:not([data-type="togglePanel"]),
#singleOutline.single-outline--collapsed > .fn__flex-1 {
    display: none !important;
}

#singleOutline.single-outline--collapsed .block__icons {
    justify-content: center;
    padding: 6px 0;
}

#singleOutline.single-outline--collapsed [data-type="togglePanel"] {
    margin: 0;
}

.single-doc-meta {
    margin-top: 6px;
    color: var(--b3-theme-on-surface);
    font-size: 12px;
    line-height: 18px;
    opacity: .72;
    user-select: none;
}

@media (max-width: 900px) {
    #singleOutline {
        width: 220px;
        min-width: 200px;
    }
}
`;
    document.head.appendChild(styleElement);
}

