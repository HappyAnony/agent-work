export const DOC_EXT = ".docj";
export const DOC_ZIP_EXT = ".docj.zip";

export function hasDocExt(s: string): boolean {
    return s.endsWith(DOC_EXT);
}

export function stripDocExt(s: string): string {
    return s.endsWith(DOC_EXT) ? s.slice(0, -DOC_EXT.length) : s;
}

export function appendDocExt(id: string): string {
    return id + DOC_EXT;
}

export function hasDocZipExt(s: string): boolean {
    return s.endsWith(DOC_ZIP_EXT);
}

export function stripDocZipExt(s: string): string {
    return s.endsWith(DOC_ZIP_EXT) ? s.slice(0, -DOC_ZIP_EXT.length) : s;
}
