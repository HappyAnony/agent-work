// Dedicated-window safety boundary, never a current workspace/page. Once a
// controlled editor has mounted, detaching it must not re-enable legacy tools.
let portable = false;
export function markPortableWindow(): void { portable = true; }
export function isPortableWindow(): boolean { return portable; }
