import * as fs from "fs";
import * as path from "path";

// Keep the original selected folder and frontend wrapper. Replace the index
// entry instead of truncating its inode (which could be a link to another file).
// The backend checks the selected directory before returning its resource DOM;
// this is not a cross-process transaction for the directory and its contents.
export const writeDesktopExportHTML = (folder: string, html: string) => {
    const temporary = path.join(folder, `.wiki-html-${Date.now()}-${Math.random().toString(36).slice(2)}`);
    let descriptor: number | undefined;
    let identity: fs.Stats | undefined;
    let failure: unknown;
    let published = false;
    try {
        descriptor = fs.openSync(temporary, "wx", 0o644);
        identity = fs.fstatSync(descriptor);
        fs.writeFileSync(descriptor, html);
        fs.fsyncSync(descriptor);
        fs.closeSync(descriptor);
        descriptor = undefined;
        fs.renameSync(temporary, path.join(folder, "index.html"));
        published = true;
    } catch (error) {
        failure = error;
    } finally {
        if (typeof descriptor !== "undefined") {
            try { fs.closeSync(descriptor); } catch (error) { failure = failure || error; }
        }
        if (identity && !published) {
            try {
                const current = fs.lstatSync(temporary);
                if (current.dev !== identity.dev || current.ino !== identity.ino) {
                    throw new Error("HTML export temporary file ownership changed");
                }
                fs.unlinkSync(temporary);
            } catch (error) {
                if ((error as NodeJS.ErrnoException).code !== "ENOENT") {
                    failure = failure || error;
                }
            }
        }
    }
    if (failure) { throw failure; }
};
