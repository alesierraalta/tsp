# The architecture corpus

`tsp probe arch` judges **one thing**: an import edge the change adds that a rule this repository
declared refuses. The corpus is the evidence that it does that, and only that.

Every row is a fixture in `internal/arch/corpus_test.go`: a module, a rule file, a diff, and either the
findings the probe must report or the silence it must keep. A rule that catches violations but also fires
on clean code is not a rule anyone can gate a pull request with, so the controls are as load-bearing as
the positives.

## What the corpus pins

| class | fixture | expected |
|---|---|---|
| `forbidden-edge` | `internal/app` imports `cmd`, refused by `forbid` with the repository's own reason | one finding at the added line, quoting that reason |
| `two-forbidden-edges` | two refused imports on two added lines | two findings, one per line, in source order |
| `javascript-relative-edge` | `web/app.ts` imports `../core/db`, refused by `forbid` | one finding at `web/app.ts:1` |
| `control-declared-edge` | the import the layer already participates in | nothing |
| `control-standard-library` | `sort` added beside an allowed import | nothing: the standard library is not a layer |
| `control-undeclared-package` | an import of a package that matches no declared layer | nothing, and the report says a package matching no layer is not judged |
| `control-untouched-line` | a refused edge that was already in the file | nothing: the change is what is judged |

## What this does not prove

- **Cycles.** The Go toolchain refuses an import cycle at build time, so a cycle a build accepts cannot
  exist in Go. A JavaScript or TypeScript cycle is *not* reported.
- **JavaScript parsing.** Specifiers are read per line (`from`, `require`, dynamic `import`), not through a
  parser, so a specifier inside a comment can cost one extra line of attention.
- **Layers nobody declared.** A package that matches no layer is skipped, not judged against a default.
- **Direction between layers.** A layer's `imports` names the import paths that *belong* to it; the only
  refusal is an explicit `forbid` entry (or `tsp probe arch`'s own reading of that list). There is no
  "layer A may only import layer B" rule, so a corpus entry for one would be fiction.

## Why so few classes

Because the probe implements one rule. Adding corpus entries for rules that do not exist would make the
corpus look thorough and mean nothing; the four controls are there to keep the one rule honest.
