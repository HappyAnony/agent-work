/**
 * Validate the exact window origin used by the portable editor's private channel.
 * @param value - Serialized origin supplied by the embedding application.
 * @returns Whether the origin identifies a supported application window.
 */
export function validEditorParentOrigin(value: string): boolean {
    if (value === "tauri://localhost" || value === "dsh-app://app") { return true; }
    let origin: URL;
    try { origin = new URL(value); }
    catch (error) { return false; }
    if (["http:", "https:"].includes(origin.protocol) && value === origin.origin && typeof location !== "undefined" && value === location.origin) { return true; }
    return origin.protocol === "http:" && ["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname) && value === origin.origin;
}
