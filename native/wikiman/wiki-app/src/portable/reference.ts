/** Read-only recognition. Native knowledge URLs are data, never instructions
 * to navigate the frame or an ambient legacy workspace. Legacy spelling is
 * accepted only to read existing stored knowledge, not rewritten into source. */
export function referenceTarget(target: Element, root: HTMLElement): {blockId:string|null}|null {
    const ref=target?.closest?.('[data-type~="block-ref"]');
    if(ref&&root.contains(ref)){
        const id=ref.getAttribute("data-id")??"";
        return{blockId:/^\d{14}-[a-z0-9]{7}$/.test(id)?id:null};
    }
    const link=target?.closest?.("[data-href],a[href],[data-wiki-native-href],[data-wiki-native-data-href]");
    if(!link||!root.contains(link)){return null;}
    const value=link.getAttribute("data-href")??link.getAttribute("href")??link.getAttribute("data-wiki-native-href")??link.getAttribute("data-wiki-native-data-href")??"";
    if(!/^(?:wiki|siyuan):/i.test(value)){return null;}
    // Match original text, not URL-normalized paths: /../ must not become a
    // valid local target by normalization. No credentials/query/fragment here.
    const match=/^(?:wiki|siyuan):\/\/blocks\/(\d{14}-[a-z0-9]{7})$/.exec(value);
    return{blockId:match?.[1]??null};
}
