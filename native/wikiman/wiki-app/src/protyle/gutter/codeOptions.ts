/** Original per-block overrides of the editor's code display defaults. */
export function codeBlockOptions(block: Element, apply: (key: string, value: string) => void): IMenu[] {
    const {editor} = window.siyuan.config;
    const options = [
        {key: "linewrap", id: "md31", defaultValue: editor.codeLineWrap},
        {key: "ligatures", id: "md2", defaultValue: editor.codeLigatures},
        {key: "linenumber", id: "md27", defaultValue: editor.codeSyntaxHighlightLineNum},
    ];
    return options.map(({key, id, defaultValue}) => {
        const override = block.getAttribute(key);
        const checked = override === "true" || override !== "false" && defaultValue;
        const label = window.siyuan.languages[id];
        return {
            id,
            iconHTML: "",
            label: `<div class="fn__flex" style="margin-bottom:4px"><span>${label}</span><span class="fn__space fn__flex-1"></span>
<input type="checkbox" class="b3-switch fn__flex-center"${checked ? " checked" : ""}></div>`,
            bind(element) {
                const input = element.querySelector("input");
                input.setAttribute("aria-label", label);
                element.addEventListener("click", event => {
                    if (event.target !== input) { input.checked = !input.checked; }
                    apply(key, String(input.checked));
                    window.siyuan.menus.menu.remove();
                });
            },
        };
    });
}
