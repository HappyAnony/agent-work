use fs2::FileExt;
use std::{env, fs::OpenOptions, process};

fn main() {
    let args: Vec<String> = env::args().collect();
    let file = OpenOptions::new().read(true).open(&args[1]).unwrap();
    let result = if args[2] == "shared" {
        FileExt::try_lock_shared(&file)
    } else {
        FileExt::try_lock_exclusive(&file)
    };
    match result {
        Ok(()) => FileExt::unlock(&file).unwrap(),
        Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => process::exit(23),
        Err(error) => panic!("lock probe failed: {error}"),
    }
}
