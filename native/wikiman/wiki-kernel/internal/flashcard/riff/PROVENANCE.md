# Original flashcard algorithm

Derived from github.com/siyuan-note/riff v0.0.0-20251022131846-228528e70754 (228528e70754), AGPL-3.0-or-later. Original copyright and license notices are retained.

Changes: inject storage and clock; remove global filesystem/logging dependencies; validate decoded identities; bound weight parsing; deep-copy cards without logging side effects. Original FSRS algorithm and MessagePack deck/card/monthly log fields are retained. Historical riff names denote the compatibility storage format.

R02 follow-up adds side-effect-free Deck.Snapshot for detached transaction preparation and repeat-review baselines. Shared add/remove/reset and review orchestration derives from the original kernel model/flashcard.go; it retains the original storage fields and FSRS scheduler.

