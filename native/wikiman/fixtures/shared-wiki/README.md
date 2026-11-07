# Shared Wiki synthetic fixtures

`valid/` is a hand-authored store, not a copy of personal notes. It includes
19-digit string page IDs, duplicate sibling ranks/names, a child single-document
page, an empty table node, and native `.docj` paragraphs/code/references plus an
SVG attachment. Tests must copy it to a temporary directory before mutations.

Tree and AST consumers should preserve unknown fields. Failure variants (broken
JSON, duplicate keys/IDs, missing parents, cycles, deleted parents and invalid
entry documents) are generated from temporary copies by the relevant tests.
No runtime index, lock, credential, or user data belongs in this fixture.
