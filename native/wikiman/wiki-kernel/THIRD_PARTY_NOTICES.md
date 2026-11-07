# Additional local source notices

The original kernel and its dependency notices retain their existing copyrights
and licenses. This file records the additional private fork introduced by R02;
it does not relicense the original kernel.

## radovskyb/watcher v1.0.7 polling backend

An existing BSD-3-Clause dependency has a narrow local source fork in
[internal/pollwatch/backend/](internal/pollwatch/backend/). Its
[provenance](internal/pollwatch/backend/PROVENANCE.md) records the upstream version,
files and local lifetime fixes; the complete copyright, redistribution conditions
and disclaimer are retained in [LICENSE](internal/pollwatch/backend/LICENSE).

Any binary distribution containing this fork must reproduce that complete
license notice. The runtime preparation script stages LICENSE and PROVENANCE.md
under `notices/pollwatch/`, the input gate requires them, and the Tauri resource
mapping includes them. This task builds only isolated test binaries, not a
distributable release; the actual packaged notices still require release QA.
