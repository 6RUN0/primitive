# Mutation testing

`make mutants` runs gremlins on a copy of the tree under `.mutants/src`, so that
a crash cannot leave a mutant in the working tree. The report is
`.mutants/report.json`.

| Run | Command | When |
| --- | --- | --- |
| Changed lines | `make mutants BASE=origin/master` | once before a merge |
| Whole tree | `make mutants BASE=` | before a release, on `master` |

The same commands run in the workflow `Mutants`, started by hand; its `base` input
is `BASE`.

`--diff` calls `git diff --merge-base $(BASE)` in the copy, so the run needs the
history of `BASE`. It sees tracked files only: a file that `git add` has not seen
is not mutated. An empty diff makes gremlins mutate everything, not nothing:
`make mutants` on `master` with `BASE=origin/master` is a full run. A path
argument next to `--diff` matches nothing and skips every mutant, so the target
passes none.

Too low a `timeout-coefficient` in `.gremlins.yaml` shows as `TIMED OUT`, which
counts as killed and hides survivors; see "Timeouts".

## Memory

A mutant can turn a loop that appends scanlines into an endless one. With `y--`
in the loop of `Rectangle.Rasterize`, `go test ./primitive` reached a 2 GiB limit
in 0.64 s, so four workers can exhaust a machine well before
`timeout-coefficient` stops them. `make mutants` therefore runs under
`ulimit -v $(MUTANTS_MEMORY_KB)`, 3 GiB per process by default: such a mutant
dies with `fatal error: out of memory`, which gremlins counts as killed. The
tests pass under this limit. Keep `MUTANTS_WORKERS x MUTANTS_MEMORY_KB` below free
memory: GitHub-hosted runners have 16 GiB; on a shared machine lower both.

## Timeouts

gremlins limits each mutant to `timeout-coefficient` times the baseline test run,
so the value has to cover the slowdown of parallel workers.

| Run of `primitive.test`, Ryzen 7 2700, go1.27.3 | Time of one process |
| --- | --- |
| One process | 2.34 s |
| 4 processes, 16 threads | 2.5 s |
| 8 processes, 16 threads | 3.4 s |
| 4 processes, pinned to 4 CPUs with `taskset -c 0-3` | 5.0 s |
| `go test -count=1 ./primitive`, `nice -n 19 GOMAXPROCS=2`, load average 17 to 43 | 11.7 s |

The worst case is 5 times the single run: at a coefficient of 5 the limit is
11.7 s, exactly that run. `timeout-coefficient: 10` is that ratio with a margin
of 2: a loaded machine does not time out and hide survivors as `TIMED OUT`, and
a mutant that loops forever costs 10 baseline runs, about 25 s. Measure again
with `go test -c ./primitive` and several copies of the binary when the test
suite grows.

## Format of the survivor list

Each survivor gets a test that kills it. A survivor no test can tell from the
original goes to the table below, one row per mutant:

- File: path from the module root.
- Expression: the mutated expression and its function. Line numbers shift, so
  they are not recorded.
- Mutation: the operator gremlins applied.
- Why equivalent: the argument that no input can show a difference. "Not triaged"
  is not a reason.

A survivor that only an input too large for a test run can show goes to "Survivors
not testable", with that input and its cost.

## Equivalent survivors

| File | Expression | Mutation | Why equivalent |
| --- | --- | --- | --- |
| `main.go` | `width >= height` in `makeThumbnail` | CONDITIONALS_BOUNDARY | At equal sides both branches compute `maxSize` for width and `max(1, side*maxSize/side)`, which is `maxSize`, for height. |
| `primitive/gif.go` | `y < box.Max.Y` in `changedRegion` | CONDITIONALS_BOUNDARY | The extra row lies outside `region.Rect`, and `Paletted.SetColorIndex` ignores a point outside its rectangle; the reads of `ColorIndexAt` have no side effects. |
| `primitive/gif.go` | `x < box.Max.X` in `changedRegion` | CONDITIONALS_BOUNDARY | Same as the row: the extra column is outside `region.Rect` and is ignored by `SetColorIndex`. |
| `primitive/svg.go` | `2*len(xs)` in `closedSVGPath` | ARITHMETIC_BASE | The expression is the capacity hint of `make`; `append` grows the slice and the path text does not depend on the capacity. |
| `primitive/svg.go` | `c.A < common` in `findCommonAlpha` | CONDITIONALS_BOUNDARY | The comparison is reached with `count == commonCount`; at `c.A == common` the assignment stores the values the variables already hold. |
| `primitive/tables.go` | `1<<32 - 1` in `maxTableWidth` | ARITHMETIC_BASE, INVERT_NEGATIVES | `(2^32 + 1) / 65025` and `(2^32 - 1) / 65025` both round down to 66051: 66051 x 65025 = 4294966275 and 66052 x 65025 = 4295031300 lie on both sides of either numerator. |

## Survivors not testable

| File | Expression | Mutation | Why not tested |
| --- | --- | --- | --- |
| `primitive/tables.go` | `size.X*size.Y > maxTablePixels` in `newPixelTables` | CONDITIONALS_BOUNDARY | Only an image of exactly 2^26 pixels (8192 x 8192) tells `>` from `>=`, and accepting it allocates 3.5 GiB of tables. The sides of the limit are pinned by `TestLargeImageGoesWithoutTables` at 8193 x 8192. |
