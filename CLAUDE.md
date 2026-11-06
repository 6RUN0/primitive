# CLAUDE.md

## Commands

```sh
make              # the binary primitive.bin; CodeQL autobuild runs plain make
make check        # the CI job: lint tidy test cover fuzz vuln build
make test         # go test -race
make cover        # fails below COVER_MIN of the Makefile; .cover/cover.html
make fuzz FUZZTIME=10m  # each target FUZZTIME long, 10s in check
make lint         # golangci-lint, actionlint, zizmor --offline (via uvx)
make vuln         # govulncheck, blocking
make mutants      # gremlins on changed lines, once before a merge; docs/mutants.md
go build -o primitive.bin .   # the binary
go test ./primitive -run TestGoldenOutput -update   # rewrite golden files
```

Tools are pinned in `tools/`, not in the root `go.mod`, so the module of a release
carries no tool dependencies: `tools/go.mod` has golangci-lint and govulncheck,
`tools/actionlint` and `tools/gremlins` have their own, because their dependencies
conflict with the first. zizmor is pinned in `tools/requirements.txt`. goreleaser
is pinned in the `version:` inputs of `.github/workflows`, which Dependabot does
not update.

## Tests

- `main_cli_test.go` runs the test binary as the CLI through `TestMain` and
  `PRIMITIVE_TEST_RUN_MAIN=1`, because only a real process shows the exit code.
- `make cover` uses `-coverprofile`: `go test` then collects the counters of those
  child processes. `-test.gocoverdir` loses them and `main` reads 0%.
- Golden files in `primitive/testdata/golden` pin the whole search. An intended
  change of the search or of `deriveWorkerSeed` needs `-update`, and the diff of
  the SVG files is the review.
- `TestSVGRenderMatchesPNG` and `TestSVGClipStopsAtFrame` need `rsvg-convert` and
  skip without it, CI has none: run them locally after a change of the SVG output.
- A failing fuzz input lands in `primitive/testdata/fuzz/<Target>/`; commit it
  with the fix, it then runs in plain `go test`.
- Comments and test docstrings name the defect a test pins, never a plan stage.

## Gotchas

- `go build` without `-o` fails: the package directory `primitive` has the name
  of the binary.
- `.gitignore` is deny-first: a new top-level file stays invisible to
  `git status` until it gets a `!/name` line, and dot files inside allowed
  directories stay denied. `.github/` allows only `workflows/` and
  `dependabot.yml`.
- Output depends on the build: FMA fuses floating-point operations in the shape
  code and in `estimateEnergy`, so `GOAMD64=v3` can change any mode for one seed,
  and `TestGoldenOutput` skips itself there and off amd64. Run `-update` only on a
  default amd64 build. Numbers are in `README.md`, "Reproducible Output".
- SVG golden files are one line each: review them with `git diff --word-diff`.
- `math/rand` reduces a seed modulo 2^31-1, and a sum of seed and index shares
  streams between adjacent seeds; `deriveWorkerSeed` exists for both.
- `SaveGIF` assumes alpha never falls between frames, which holds for
  `Model.Frames`: unchanged pixels are transparent over the frame before.
- `gremlins --diff` with an empty diff mutates the whole tree.
- `Makefile` must keep `build` as the default goal: the CodeQL autobuilder calls
  `make` without a target, and `check` would run tests and fuzzing under its
  trace.
- `release.yml` attests after `goreleaser release`: a failed attestation leaves
  a published release and image without provenance, and the job cannot rerun until
  that release is deleted.
- `ci.yml` keeps a fuzz failure as the artifact `fuzz-failures`: commit the file
  to `primitive/testdata/fuzz/<Target>/`, a new random search may not repeat it.
- Release tags are `vX.Y.Z` or `vX.Y.Z-rcN`; `make release-gate` wants the tag on
  the head of `master` and a green `ci.yml` run for that commit.
