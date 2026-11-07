// Keep modal actions anchored to the block that opened the candidate.
export function captureScopedHintRange(block: HTMLElement, range: Range) {
    const initial = block.innerHTML;
    const node = range.startContainer;
    if (node !== range.endContainer || node.nodeType !== Node.TEXT_NODE) { return; }
    const start = range.startOffset, end = range.endOffset;
    const walker = document.createTreeWalker(block, NodeFilter.SHOW_TEXT);
    let index = 0;
    while (walker.nextNode() && walker.currentNode !== node) { index++; }
    if (walker.currentNode !== node) { return; }
    return (protyle: IProtyle) => {
        if (protyle.disabled || !protyle.wysiwyg.element.contains(block) ||
            block.innerHTML !== initial || !block.contains(node)) { return; }
        const clone = block.cloneNode(true) as HTMLElement;
        const copy = document.createTreeWalker(clone, NodeFilter.SHOW_TEXT);
        for (let i = 0; i <= index; i++) { copy.nextNode(); }
        const selection = document.createRange();
        selection.setStart(copy.currentNode, start);
        selection.setEnd(copy.currentNode, end);
        selection.deleteContents();
        return {clone, selection};
    };
}
