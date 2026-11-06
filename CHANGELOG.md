# Changelog

Russian: [CHANGELOG.ru.md](CHANGELOG.ru.md).

The format is based on [Keep a Changelog
1.1.0](https://keepachangelog.com/en/1.1.0/). The baseline of this file is the
state of `master` before the first release of this fork. Entries marked
"Breaking" need action from a user of the command line or of the Go package.

## [Unreleased]

## [0.1.0-rc1] - 2026-11-06

### Added

- `-seed`: a run is repeatable. Equal input, flags, `-seed` and `-j` give the
  same bytes within one build; `-v` prints the seed. Limits are in the README,
  section "Reproducible Output".
- `-nc`: random shapes tried before each hill climb, also as `Model.Candidates`
  and `DefaultCandidates`. `-nc 500` restores the former number of candidates.
- `-version`: prints the release number, or the module version in a build
  without a release.
- Release archives for Linux, macOS, Windows and FreeBSD on amd64 and arm64,
  with `checksums.txt` and build provenance attestations, and a distroless image
  `ghcr.io/6run0/primitive` for linux/amd64 and linux/arm64 that runs as a
  non-root user.

### Changed

- Breaking: the module path is `github.com/6RUN0/primitive`, not `primitive`.
  Imports use `github.com/6RUN0/primitive/primitive`. The command builds the
  package of this repository; it used to import the package of another fork, so
  changes in `primitive/` did not reach the binary.
- Breaking: `NewModel` and `NewWorker` take a final `seed int64` argument, and
  `MakeHexColor` returns `(Color, error)`. Each worker derives its own random
  stream from the seed.
- Breaking: Go 1.27.0 or newer is required, `go.mod` names the toolchain
  `go1.27.3`.
- Breaking: the command rejects with exit code 1, before the search, what it
  used to accept, or what failed only after the search or with a panic: `-a`
  outside 0 to 255, `-bg` other than 3, 4, 6 or 8 hex digits, `-m` outside 0 to
  8, `-mqw` not a finite number above 0, `-n`, `-nc`, `-nth` or `-s` below 1,
  `-int` outside 0 to 65535, `-s` above 65535 for GIF and JPEG, an unknown
  output extension, and a malformed `%` pattern in a path. An input without
  pixels is rejected after reading. Scripts that passed such values now stop
  with an error.
- Breaking: equal input and flags give another picture than older builds. The
  search changed as described below, and a picture of an older build cannot be
  reproduced with `-seed`.
- A search takes 8 hill climbs per shape for any `-j`, starts them from 125
  candidates instead of 500, and ranks candidates by a fast estimate, except for
  `-m 6` and for transparent or very large images. CPU falls 73 to 96% and wall
  time 67 to 92%, at an rmse change of -0.09 to +2.93%. Measured on 4 pictures,
  `-n 100 -a 128 -r 256 -s 1024`, seeds 1 to 4, go1.27.3, Ryzen 7 2700, modes 0,
  1, 3 and 7, `-j` 1 and 16; the table is in the README, section "CPU Load".
  `-nc 500` lowers the rmse cost at about twice the CPU. Above 8 workers extra
  workers stay idle.
- Memory: up to 2^26 pixels the search keeps tables of 56 bytes per pixel, 5.3
  GiB peak at 8192x8192. A larger input runs without tables, with no warning,
  and the first shape is not done after 165 s at 8193x8192. See the README,
  section "Large Inputs".
- GIF output no longer needs ImageMagick: the program quantizes to 255 colors
  from the last frame and writes frame differences. On `lenna`,
  `-n 100 -a 128 -m 1 -j 8 -seed 5 -r 256 -s 1024`, 49 frames, on the frames of
  one earlier search: the file falls from 3.7 MB to 210 KB and writing from 19.2
  s to 0.46 s; rmse to the PNG frames moves from 0.00224 to 0.00295 on average
  and from 0.00598 to 0.00403 at worst. The current search gives other frames:
  the same command now writes 226 KB, 49 frames. Colors that only earlier frames
  show are approximated.
- SVG output is compact: 2 decimals for coordinates, 3 for opacity, one line,
  shapes clipped to the frame. On `lenna`, `-n 200 -s 1024 -seed 7 -j 8`, `-r`
  128 and 256, size falls 50% for `-m 6`, 36 to 38% for `-m 7` and `-m 8`, 23%
  for `-m 0`, 17 to 18% for `-m 1`. The clip costs a fixed amount, so after
  `gzip -9` modes 2 to 5 grow by 0.2 to 3.2%. At a very large scale the rounding
  is visible: at `-r 16 -s 4096` the render differs from the PNG by 0.0058
  against 0.0045 with 6 decimals.
- `-r` resizes with `golang.org/x/image/draw` instead of `nfnt/resize`. Pixels
  differ by 0.39 to 0.49% of the full range on `lenna` at `-r` 32 to 256. At
  `-r 256` 8.4% of the source sizes from 1x1 to 1200x1200 get another width, by
  one pixel, all of them portrait images wider than the limit.
- `golang.org/x/image` is v0.47.0.

### Fixed

- SVG did not match the raster output: shapes were shifted by half a pixel, and
  curves lost their opacity and round ends. The rendered SVG differed from the
  PNG of the same run by rmse 0.027 to 0.15; it differs by 0.0023 to 0.0033
  (conditions as in the SVG entry of "Changed", rsvg-convert 2.62.3). A
  background with alpha keeps it, and shapes no longer spill over the frame.
- SVG `viewBox` was wrong at a fractional scale, `333x222` instead of `256x171`
  at `-s 1000`, and the picture did not fill the frame; the height was also
  truncated, 170 instead of 171 at `-s 1024`.
- A GIF could end before the last shapes. It always ends with the finished
  picture: rmse of the last frame to the PNG on `lenna`, `-s 256 -seed 7 -j 4`,
  falls from 0.00706 to 0.00569 at `-n 100` and from 0.00904 to 0.00324 at
  `-a 200 -m 8 -n 60`.
- Panics on valid input: `-s` smaller than the width of the input (integer
  divide by zero), an input whose bounds do not start at zero such as a GIF
  frame with an offset.
- Panics on invalid values, which are now errors: `-s` below 1, `-nth 0` with a
  `%d` path.
- `-m 7` limited the vertical radius by the width of the image instead of its
  height.
- The average background color of an input with an offset was wrong,
  `{8 4 2 255}` instead of `{200 100 50 255}`, with no error.
- `MakeHexColor` turned malformed values such as `0X00` into a color with alpha
  4095.
- `Model.SVG` panicked on a model without shapes, `AverageImageColor` on an
  image without pixels.
- An error of closing an output file is reported with exit code 1 instead of
  being lost, and a failed encoder leaves an existing file as it was.

### Removed

- Breaking: `Heatmap`, `NewHeatmap` and `Worker.Heatmap`, which cost 8 bytes per
  pixel per worker and had no effect on the output.
- Breaking: `Anneal` and `PreAnneal`. The command never used them.
  `Worker.Buffer` and `SaveGIFImageMagick` are gone as well.
- Breaking: the flag `-ii`, which was accepted and had no effect. Passing it is
  now an error with exit code 2.
- The Python bot, the helper scripts of `scripts/` and `build.sh`. Build with
  `go build -o primitive.bin .` or `make`.
- The pictures `examples/*-97-mean3.png` and their parameter files
  `examples/*-97-parm*.txt`. `make examples` writes current output to
  `examples/output/`, and the README shows it.

### Security

- Releases build with go1.27.3: `govulncheck` finds 13 vulnerabilities of the
  standard library in go1.27.0, fixed in 1.27.2 and 1.27.3, none of them reached
  by this code.
- The program starts no external process, so its output no longer depends on
  what the `PATH` resolves to.
- Release archives carry build provenance attestations, checked with
  `gh attestation verify`. The image has none.

[Unreleased]: https://github.com/6RUN0/primitive/compare/v0.1.0-rc1...master
[0.1.0-rc1]: https://github.com/6RUN0/primitive/releases/tag/v0.1.0-rc1
