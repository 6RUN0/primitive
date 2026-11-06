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

A survivor that only an input too large for a test run, or an input the search
never produces, can show goes to "Survivors not testable", with that input and
its cost or the reason it does not occur.

## Equivalent survivors

| File | Expression | Mutation | Why equivalent |
| --- | --- | --- | --- |
| `main.go` | `width >= height` in `makeThumbnail` | CONDITIONALS_BOUNDARY | At equal sides both branches compute `maxSize` for width and `max(1, side*maxSize/side)`, which is `maxSize`, for height. |
| `primitive/core.go` | `i+0` in `drawLines` (both reads and writes of the red channel) | ARITHMETIC_BASE | `i-0` and `i+0` are the same index. |
| `primitive/core.go` | `i < len(im.Pix)` in `isOpaque` | CONDITIONALS_BOUNDARY | `i` runs over 3, 7, 11, ... and `len(im.Pix)` is a multiple of 4, so `i` never equals the length. |
| `primitive/ellipse.go` | `y1 >= h` in the early exit of `Ellipse.Rasterize` | CONDITIONALS_BOUNDARY | The exit only skips rows that the guards `y1 < h` and `y2 < h` before the two appends skip anyway; a row at `h` is not appended either way. |
| `primitive/ellipse.go` | `y2 >= h` in the early exit of `Ellipse.Rasterize` | CONDITIONALS_BOUNDARY | Same as `y1 >= h`: the exit is a shortcut and the append guards decide. |
| `primitive/ellipse.go` | `y1 < h` in the first append of `Ellipse.Rasterize` | CONDITIONALS_BOUNDARY | `y1 == h` makes `y2 >= y1 >= h`, so the early exit has skipped the row before the guard is read. |
| `primitive/ellipse.go` | `x1 < 0` in `Ellipse.Rasterize` | CONDITIONALS_BOUNDARY | At `x1 == 0` the assignment stores 0, the value it already has. |
| `primitive/ellipse.go` | `i+0` in `RotatedEllipse.Rasterize` | ARITHMETIC_BASE | `i-0` and `i+0` are the same value. |
| `primitive/gif.go` | `y < box.Max.Y` in `changedRegion` | CONDITIONALS_BOUNDARY | The extra row lies outside `region.Rect`, and `Paletted.SetColorIndex` ignores a point outside its rectangle; the reads of `ColorIndexAt` have no side effects. |
| `primitive/gif.go` | `x < box.Max.X` in `changedRegion` | CONDITIONALS_BOUNDARY | Same as the row: the extra column is outside `region.Rect` and is ignored by `SetColorIndex`. |
| `primitive/model.go` | `aspect >= 1` in `NewModel` | CONDITIONALS_BOUNDARY | At `aspect == 1` both branches give `size` for width and height and the scale `size / side`. |
| `primitive/optimize.go` | `step++` in `HillClimb` | INCREMENT_DECREMENT | `step` is never read: its only use is a commented-out log line. The variable is dead code. |
| `primitive/polygon.go` | `a < p.Order` in `Polygon.Valid` | CONDITIONALS_BOUNDARY | For `Order >= 1` the extra pass `a == Order` reads the triple of `a == 0` again and compares its sign with itself. |
| `primitive/polygon.go` | `a + 0` in `Polygon.Valid` | ARITHMETIC_BASE | `a-0` and `a+0` are the same index. |
| `primitive/rectangle.go` | `x1 > x2` in `Rectangle.bounds` | CONDITIONALS_BOUNDARY | Swapping two equal values changes nothing. |
| `primitive/rectangle.go` | `y1 > y2` in `Rectangle.bounds` | CONDITIONALS_BOUNDARY | Same as `x1 > x2`. |
| `primitive/rectangle.go` | `a < b` in `RotatedRectangle.Valid` | CONDITIONALS_BOUNDARY | Swapping two equal sides changes nothing. |
| `primitive/scanline.go` | `w-1` as the upper limit of `line.X1` in `cropScanlines` | ARITHMETIC_BASE, INVERT_NEGATIVES | The check `line.X1 >= w` before it has skipped every line with `X1 > w-1`, so the upper limit is never reached. |
| `primitive/svg.go` | `2*len(xs)` in `closedSVGPath` | ARITHMETIC_BASE | The expression is the capacity hint of `make`; `append` grows the slice and the path text does not depend on the capacity. |
| `primitive/svg.go` | `c.A < common` in `findCommonAlpha` | CONDITIONALS_BOUNDARY | The comparison is reached with `count == commonCount`; at `c.A == common` the assignment stores the values the variables already hold. |
| `primitive/tables.go` | `1<<32 - 1` in `maxTableWidth` | ARITHMETIC_BASE, INVERT_NEGATIVES | `(2^32 + 1) / 65025` and `(2^32 - 1) / 65025` both round down to 66051: 66051 x 65025 = 4294966275 and 66052 x 65025 = 4295031300 lie on both sides of either numerator. |
| `primitive/triangle.go` | `a1 > minDegrees` in `Triangle.Valid` | CONDITIONALS_BOUNDARY | An angle of exactly 15 degrees needs a rational tangent equal to tan 15 = 2 - sqrt 3, which is irrational; at the coordinates of an image the nearest integer triangles differ by far more than the rounding of `math.Acos`. |
| `primitive/triangle.go` | `a2 > minDegrees` in `Triangle.Valid` | CONDITIONALS_BOUNDARY | Same as `a1 > minDegrees`. |
| `primitive/triangle.go` | `a3 > minDegrees` in `Triangle.Valid` | CONDITIONALS_BOUNDARY | Same as `a1 > minDegrees`: `a3` is `180 - a1 - a2`, the third angle of an integer triangle. |
| `primitive/triangle.go` | `y2 > y3` in `rasterizeTriangle` | CONDITIONALS_BOUNDARY | The swap of two vertices with equal rows only relabels them, and with all three rows equal the output depends on `x1` alone: `TestTriangleRasterizeIgnoresVertexOrderOnSmallGrid` shows the same scanlines for all six orders of every triangle with area on a 4x4 grid. |
| `primitive/triangle.go` | `a > b` in `rasterizeTriangleBottom` | CONDITIONALS_BOUNDARY | Swapping two equal ends of a scanline changes nothing. |
| `primitive/triangle.go` | `a > b` in `rasterizeTriangleTop` | CONDITIONALS_BOUNDARY | Same as in `rasterizeTriangleBottom`. |
| `primitive/util.go` | `x < lo` in `clamp` | CONDITIONALS_BOUNDARY | At `x == lo` both branches return an equal value; only the sign of zero can differ, and coordinates start at `+0` and never reach `-0`. |
| `primitive/util.go` | `x > hi` in `clamp` | CONDITIONALS_BOUNDARY | At `x == hi` both branches return an equal value, as for `x < lo`. |
| `primitive/util.go` | `x < lo` in `clampInt` | CONDITIONALS_BOUNDARY | Same as in `clamp`. |
| `primitive/util.go` | `x > hi` in `clampInt` | CONDITIONALS_BOUNDARY | Same as in `clamp`. |
| `primitive/util.go` | `a < b` in `minInt` | CONDITIONALS_BOUNDARY | At `a == b` both branches return the same value. |
| `primitive/util.go` | `a > b` in `maxInt` | CONDITIONALS_BOUNDARY | At `a == b` both branches return the same value. |
| `primitive/util.go` | `y < h` in `AverageImageColor` | CONDITIONALS_BOUNDARY | `RGBAAt` returns the zero color outside the image, which adds 0 to every sum; the divisor stays `w*h`. |
| `primitive/util.go` | `x < w` in `AverageImageColor` | CONDITIONALS_BOUNDARY | `RGBAAt` returns the zero color outside the image, which adds 0 to every sum; the divisor stays `w*h`. |

## Survivors not testable

| File | Expression | Mutation | Why not tested |
| --- | --- | --- | --- |
| `primitive/tables.go` | `size.X*size.Y > maxTablePixels` in `newPixelTables` | CONDITIONALS_BOUNDARY | Only an image of exactly 2^26 pixels (8192 x 8192) tells `>` from `>=`, and accepting it allocates 3.5 GiB of tables. The sides of the limit are pinned by `TestLargeImageGoesWithoutTables` at 8193 x 8192. |
| `primitive/triangle.go` | `y1 > y3` in `rasterizeTriangle` | CONDITIONALS_BOUNDARY | Only a triangle with all three vertices on one row tells the mutant from the original, by the `x` of its single pixel. `Triangle.Valid` rejects such a triangle, so the search never rasterizes it and its output has no meaning to pin. With rows that differ the swap only relabels vertices of equal rows, see `y2 > y3`. |
| `primitive/triangle.go` | `y1 > y2` in `rasterizeTriangle` | CONDITIONALS_BOUNDARY | Only a triangle with all three vertices on one row tells the mutant from the original, by the `x` of its single pixel. `Triangle.Valid` rejects such a triangle, so the search never rasterizes it and its output has no meaning to pin. With rows that differ the swap only relabels vertices of equal rows, see `y2 > y3`. |
