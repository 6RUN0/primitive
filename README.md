# Primitive Pictures

Reproducing images with geometric primitives.

### How it Works

A target image is provided as input. The algorithm tries to find the single most optimal shape that can be drawn to minimize the error between the target image and the drawn image. It repeats this process, adding *one shape at a time*. Around 50 to 200 shapes are needed to reach a result that is recognizable yet artistic and abstract.

### Primitive for macOS

Now available as a native Mac application!

https://primitive.lol/

### Command-line Usage

Run it on your own images! First, [install Go](https://golang.org/doc/install) 1.27 or newer.

    go install github.com/6RUN0/primitive@latest
    primitive -i input.png -o output.png -n 100

To build from a source checkout:

    go build -o primitive.bin .
    ./primitive.bin -i input.png -o output.png -n 100

The `-o primitive.bin` argument is required: a plain `go build` fails with `build output "primitive" already exists and is a directory`, because the package directory has the name of the binary.

Released binaries for Linux, macOS, Windows and FreeBSD are on the [releases page](https://github.com/6RUN0/primitive/releases); each archive listed in `checksums.txt` has a build provenance attestation:

    gh attestation verify primitive_<version>_linux_amd64.tar.gz --repo 6RUN0/primitive

To run the image, mount a directory and pass your user, because the image runs as user 65532 and cannot write to a directory it does not own (the program then exits with code 1):

    docker run --rm -u "$(id -u):$(id -g)" -v "$PWD:/work" ghcr.io/6run0/primitive -i /work/input.png -o /work/output.gif -n 100

Small input images should be used (like 256x256px). You don't need the detail anyway and the code will run faster.

| Flag | Default | Description |
| --- | --- | --- |
| `i` | n/a | input file, `-` reads standard input |
| `o` | n/a | output file, the extension selects the format and any other than those below is rejected before the search starts; `-` writes SVG to standard output |
| `n` | n/a | number of shapes |
| `m` | 1 | mode: 0=combo, 1=triangle, 2=rect, 3=ellipse, 4=circle, 5=rotatedrect, 6=beziers, 7=rotatedellipse, 8=polygon; any other value is rejected |
| `mqw` | 0.5 | maximum stroke width of `-m 6` curves in pixels of the resized input; widths start at 0.5, so a value from 0 to 0.5 has no effect; must be a finite number above 0 |
| `rep` | 0 | add N extra shapes each iteration with reduced search (mostly good for beziers) |
| `nth` | 1 | save every Nth frame (only when `%d` is in output path), must be above 0 |
| `int` | 50 | interval between frames of a gif in hundredths of a second, from 0 to 65535, any other value is rejected (works only with .gif output); the last frame stays for 2.5 s |
| `r` | 256 | resize large input images to this size before processing, `0` keeps the full size; see Large Inputs |
| `s` | 1024 | output image size along the longer side, must be above 0; GIF and JPEG outputs allow up to 65535, a larger value is rejected before the search starts |
| `a` | 128 | color alpha from 0 to 255 (use `0` to let the algorithm choose alpha for each shape); any other value is rejected |
| `bg` | avg | starting background color: `rgb`, `rgba`, `rrggbb` or `rrggbbaa` hex digits with an optional leading `#`; any other value is rejected |
| `j` | 0 | number of parallel workers (default uses all cores); a search runs 8 hill climbs in total, so above 8 extra workers stay idle and the output stops changing |
| `nc` | 125 | random shapes tried before each hill climb; must be above 0; more costs CPU and gains little past 125 |
| `seed` | 0 | random seed, `0` picks a new one for each run (`-v` prints it) |
| `ma` | 0 | stop adding shapes once the score (root-mean-square error from 0 to 1, lower is better) is at or below this value |
| `mp` | 100 | stop adding shapes once the score is this many percent lower than the score of the plain background |
| `v` | off | verbose output |
| `vv` | off | very verbose output |
| `version` | off | print the version and exit: the release number, the module version in a `go install` or a `go build` of a clone, `devel` in a build without version control data |

### Reproducible Output

Equal input, flags, `-seed` and `-j` repeat the output byte for byte within one build of the program; this is promised only for one binary. A different `-j` gives a different picture, except that all values from 8 up give one, and the default `-j` follows the number of cores, so set both flags explicitly.

Two builds can disagree for one seed, because Go fuses floating-point multiply and add where the target CPU has an instruction for it. This affects shape geometry in some modes and the estimate that ranks candidates in all modes except `-m 6`, so no mode is promised to match across builds. Measured on go1.27.3 for linux/amd64, comparing `GOAMD64=v3` with the default `v1`: `lenna`, `owl`, `monalisa` and `pyramids` at `-r 64 -n 30 -j 1 -s 128`, seeds 1 to 12, gave another PNG in 12 of 48 runs for `-m 0`, 1 for `-m 1`, 35 for `-m 5`, 14 for `-m 6` and none for the other modes; the count depends on the picture. Released amd64 binaries and images use `v1`. Whether arm64 differs from amd64 has not been measured; `TestGoldenOutput` skips itself on arm64 and on `GOAMD64=v3`.

### Large Inputs

`-r 0` or a large `-r` trades memory for detail. Up to 2^26 pixels (8192x8192) the search works on tables of 56 bytes per pixel, and only the part of the time that follows the shape outline grows with the size. Above 2^26 pixels, or above 66051 pixels of width, there are no tables, every candidate costs time in proportion to its area, and the program prints nothing for minutes.

| Input | Tables | CPU | Peak memory |
| --- | --- | --- | --- |
| 5000x5000, `-n 3 -j 1` | yes | 9.4 s | 1.7 GiB |
| 8192x8192, `-n 3 -j 1` | yes | 39 s | 5.3 GiB |
| 8193x8192, `-n 1 -j 1` | no | first shape not done after 165 s | not reached |

Measured on go1.27.3, Ryzen 7 2700, a smooth picture scaled up, `-s 64`, `-seed 1`, with other load on the machine.

### Input Formats

PNG, JPEG, GIF (first frame only) and WebP. Transparent pixels count as black when the average background color is computed. An image without pixels is rejected.

### Output Formats

Depending on the output filename extension provided, you can produce different types of output.

- `PNG`: raster output
- `JPG`: raster output
- `SVG`: vector output on one line, see [SVG Output](#svg-output)
- `GIF`: animated output showing shapes being added and ending with the finished picture, with up to 255 colors taken from the last frame; colors that only earlier frames show are approximated, a pixel below half opacity becomes transparent, and any other pixel shows its color fully opaque

For PNG, JPG and SVG outputs, you can also include `%d`, `%03d`, etc. in the filename. In this case, each frame will be saved separately. A GIF stays one file, named with the number of its last frame. A path with `%` must be a valid pattern with one number, a literal percent sign is written `%%`; anything else is rejected before the search starts.

You can use the `-o` flag multiple times. This way you can save both a PNG and an SVG, for example.

### SVG Output

The `viewBox` has the size of the resized input, not of `-s`. Fractional coordinates of `-m 6`, `-m 7`, `-m 8` and `-m 0` keep 2 decimals in those units, so a point can move by 0.005 x scale output pixels, where scale is `-s` divided by the longer side of the input. Opacity keeps 3 decimals. Shapes are clipped to the frame, and a background with alpha keeps it.

Measured on `examples/lenna.png`, `-n 200 -s 1024 -seed 7 -j 8`, go1.27.3, rsvg-convert 2.62.3: the rendered SVG differs from the PNG of the same run by a root-mean-square error of 0.0023 to 0.0033 of the full range at `-r 128` and `-r 256`. At `-r 16 -s 4096`, scale 256, the error is at most 0.0058 against 0.0045 with 6 decimals.

In those runs 200 shapes take 9 to 23 KB, 2.4 to 6.3 KB after `gzip -9`.

### Progression

This GIF demonstrates the iterative nature of the algorithm, attempting to minimize the mean squared error by adding one shape at a time. (Use a ".gif" output file to generate one yourself!)

<img src="https://www.michaelfogleman.com/static/primitive/examples/monalisa.3.2000.gif" width="440"/> <img src="https://www.michaelfogleman.com/static/primitive/examples/monalisa-original.png" width="440"/>

### Static Animation

Since the algorithm has a random component to it, you can run it against the same input image multiple times to bring life to a static image.

![Pencils](https://www.michaelfogleman.com/static/primitive/examples/pencils.gif)

### Creative Constraints

If you're willing to dabble in the code, you can enforce constraints on the shapes to produce even more interesting results. Here, the rectangles are constrained to point toward the sun in this picture of a pyramid sunset.

![Pyramids](https://www.michaelfogleman.com/static/primitive/examples/pyramids.png)

### Shape and Iteration Comparison Matrix

The matrix below shows triangles, ellipses and rectangles at 50, 100 and 200 iterations each.

![Matrix](http://i.imgur.com/H5NYpL4.png)

### How it Works, Part II

Say we have a `Target Image`. This is what we're working towards recreating. We start with a blank canvas, but we fill it with a single solid color. Currently, this is the average color of the `Target Image`. We call this new blank canvas the `Current Image`. Now, we start evaluating shapes. To evaluate a shape, we draw it on top of the `Current Image`, producing a `New Image`. This `New Image` is compared to the `Target Image` to compute a score. We use the [root-mean-square error](https://en.wikipedia.org/wiki/Root-mean-square_deviation) for the score.

    Current Image + Shape => New Image
    RMSE(New Image, Target Image) => Score

The shapes are generated randomly. We can generate a random shape and score it. Then we can mutate the shape (by tweaking a triangle vertex, tweaking an ellipse radius or center, etc.) and score it again. If the mutation improved the score, we keep it. Otherwise we rollback to the previous state. Repeating this process is known as [hill climbing](https://en.wikipedia.org/wiki/Hill_climbing). Hill climbing is prone to getting stuck in local minima, so we actually do this many different times with several different starting shapes. We can also generate N random shapes and pick the best one before we start hill climbing. [Simulated annealing](https://en.wikipedia.org/wiki/Simulated_annealing) is another good option, but in my tests I found the hill climbing technique just as good and faster, at least for this particular problem.

Once we have found a good-scoring shape, we add it to the `Current Image`, where it will remain unchanged. Then we start the process again to find the next shape to draw. This process is repeated as many times as desired.

### Primitives

The following primitives are supported:

| `-m` | Primitive | SVG element |
| --- | --- | --- |
| 0 | Combo: a mix of the types below in a single image | one of the elements below per shape |
| 1 | Triangle | `<path>` |
| 2 | Rectangle (axis-aligned) | `<rect>` |
| 3 | Ellipse (axis-aligned) | `<ellipse>` |
| 4 | Circle | `<ellipse>` |
| 5 | Rotated Rectangle | `<rect>` with `transform` |
| 6 | Quadratic Bezier curve, stroked and not filled; width set by `-mqw` | `<path>` with `stroke` |
| 7 | Rotated Ellipse | `<ellipse>` with `transform` |
| 8 | Polygon of 4 vertices, may cross itself | `<path>` |

More shapes can be added by implementing the following interface. `SVG` gets the color attributes of the shape and returns one element. `Model.SVG` gives `fill` to every shape except `*Quadratic`, so a new stroked shape needs a change there as well.

```go
type Shape interface {
	Rasterize() []Scanline
	Copy() Shape
	Mutate()
	Draw(dc *gg.Context, scale float64)
	SVG(attrs string) string
}
```

### Features

- [Hill Climbing](https://en.wikipedia.org/wiki/Hill_climbing) from multiple random shapes for optimization
- Scanline rasterization of shapes in pure Go (preferable for implementing the features below)
- Optimal color computation based on affected pixels for each shape (color is directly computed, not optimized for)
- Partial image difference for faster scoring (only pixels that change need be considered)
- Anti-aliased output rendering

### Inspiration

This project was originally inspired by the popular and excellent work of Roger Johansson - [Genetic Programming: Evolution of Mona Lisa](https://rogeralsing.com/2008/12/07/genetic-programming-evolution-of-mona-lisa/). Since seeing that article when it was quite new, I've tinkered with this problem here and there over the years. But only now am I satisfied with my results.

It should be noted that there are significant differences in my implementation compared to Roger's original work. Mine is not a genetic algorithm. Mine only operates on one shape at a time. Mine is much faster (AFAIK) and supports many types of shapes.

### Examples

Here are more examples from interesting photos found on Flickr.

![Example](https://www.michaelfogleman.com/static/primitive/examples/29167683201.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/26574286221.200.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/15011768709.200.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/27540729075.200.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/28896874003.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/20414282102.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/15199237095.200.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/11707819764.200.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/18270231645.200.128.4.3.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/15705764893.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/25213252889.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/15015411870.200.128.4.3.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/25766500104.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/27471731151.50.128.4.1.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/11720700033.200.128.4.3.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/18782606664.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/21374478713.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/15196426112.200.128.4.5.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/24696847962.png)
![Example](https://www.michaelfogleman.com/static/primitive/examples/18276676312.100.128.4.1.png)
