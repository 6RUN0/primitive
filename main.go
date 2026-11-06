// Command primitive reproduces an image with geometric shapes and writes it as a
// raster picture, an SVG file or an animated GIF.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/6RUN0/primitive/primitive"
)

// version is set at link time: -ldflags "-X main.version=1.2.3".
var version = "devel"

// Limits of the output formats: GIF and JPEG store sizes, and GIF delays, in 16 bits.
// The encoders report a size only after the whole search, image/gif truncates a delay.
const (
	maxFrameInterval = 65535
	maxOutputSide    = 65535
)

var (
	Input        string
	Outputs      flagArray
	Background   string
	Configs      shapeConfigArray
	Alpha        int
	InputSize    int
	OutputSize   int
	MaxQuadWidth float64
	Mode         int
	Workers      int
	Candidates   int
	Max          float64
	Maxpct       float64
	Nth          int
	Repeat       int
	V, VV        bool
	Interval     int
	Seed         int64
	ShowVersion  bool
)

type flagArray []string

func (i *flagArray) String() string {
	return strings.Join(*i, ", ")
}

func (i *flagArray) Set(value string) error {
	*i = append(*i, value)
	return nil
}

type shapeConfig struct {
	Count        int
	Mode         int
	Alpha        int
	Repeat       int
	MaxQuadWidth float64
}

type shapeConfigArray []shapeConfig

func (i *shapeConfigArray) String() string {
	return ""
}

func (i *shapeConfigArray) Set(value string) error {
	n, _ := strconv.ParseInt(value, 0, 0)
	*i = append(*i, shapeConfig{int(n), Mode, Alpha, Repeat, MaxQuadWidth})
	return nil
}

func init() {
	flag.StringVar(&Input, "i", "", "input image path")
	flag.Var(&Outputs, "o", "output image path")
	flag.Var(&Configs, "n", "number of primitives")
	flag.StringVar(&Background, "bg", "", "background color (hex)")
	flag.IntVar(&Alpha, "a", 128, "alpha value from 0 to 255, 0 lets the search choose it per shape")
	flag.IntVar(&InputSize, "r", 256, "resize large input images to this size only if larger than specified")
	flag.IntVar(&OutputSize, "s", 1024, "output image size")
	flag.IntVar(&Mode, "m", 1, "0=combo 1=triangle 2=rect 3=ellipse 4=circle 5=rotatedrect 6=beziers 7=rotatedellipse 8=polygon")
	flag.Float64Var(&MaxQuadWidth, "mqw", 0.5, "maximum size of quadratic lines")
	flag.IntVar(&Interval, "int", 50, "interval between frame in a gif")
	flag.IntVar(&Workers, "j", 0, "number of parallel workers (default uses all cores)")
	flag.IntVar(&Candidates, "nc", primitive.DefaultCandidates, "random shapes tried before each hill climb, more is slower")
	flag.Float64Var(&Max, "ma", 0, "target score to stop adding primitives (default 0)")
	flag.Float64Var(&Maxpct, "mp", 100, "target score in % to stop adding primitives")
	flag.IntVar(&Nth, "nth", 1, "save every Nth frame (put \"%d\" in path)")
	flag.IntVar(&Repeat, "rep", 0, "add N extra shapes per iteration with reduced search")
	flag.BoolVar(&V, "v", false, "verbose")
	flag.BoolVar(&VV, "vv", false, "very verbose")
	flag.BoolVar(&ShowVersion, "version", false, "print the version and exit")
	flag.Int64Var(&Seed, "seed", 0, "random seed, equal seed and -j repeat the output (default 0 picks a random seed)")
}

func errorMessage(message string) bool {
	fmt.Fprintln(os.Stderr, message)
	return false
}

func isValidAlpha(alpha int) bool {
	return alpha >= 0 && alpha <= 255
}

// outputExtension returns the lower-case extension that selects the format of
// output; "-" is standard output, which gets SVG.
func outputExtension(output string) string {
	if output == "-" {
		return ".svg"
	}
	return strings.ToLower(filepath.Ext(output))
}

// resolveVersion returns the version set at link time, else the one the Go tool
// recorded in the binary: go install passes no ldflags. hasInfo is the second result
// of debug.ReadBuildInfo, "(devel)" stands for a build that recorded none.
func resolveVersion(linked string, info *debug.BuildInfo, hasInfo bool) string {
	if linked != "devel" || !hasInfo || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return linked
	}
	return info.Main.Version
}

func isValidMode(mode int) bool {
	return mode >= int(primitive.ShapeTypeAny) && mode <= int(primitive.ShapeTypePolygon)
}

// isValidQuadWidth rejects what the clamp of Quadratic.Mutate cannot limit: a value
// below its lower bound of 0.5 stays usable, a negative one becomes the stroke width.
func isValidQuadWidth(width float64) bool {
	return width > 0 && !math.IsInf(width, 1)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// makeThumbnail fits source into a maxSize x maxSize square preserving the
// aspect ratio. An image that already fits, or has no pixels, is returned as is,
// so the result is never larger than the source. maxSize must be positive.
func makeThumbnail(source image.Image, maxSize int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if (width <= maxSize && height <= maxSize) || width == 0 || height == 0 {
		return source
	}
	// max keeps a very elongated image from collapsing to a zero-sized side.
	if width >= height {
		width, height = maxSize, max(1, height*maxSize/width)
	} else {
		width, height = max(1, width*maxSize/height), maxSize
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, width, height))
	// draw.BiLinear widens its kernel when downscaling and averages all covered
	// source pixels; draw.ApproxBiLinear samples only four of them and aliases.
	draw.BiLinear.Scale(thumbnail, thumbnail.Bounds(), source, bounds, draw.Src, nil)
	return thumbnail
}

func main() {
	flag.Parse()
	if ShowVersion {
		info, hasInfo := debug.ReadBuildInfo()
		fmt.Println(resolveVersion(version, info, hasInfo))
		return
	}
	ok := true
	if Input == "" {
		ok = errorMessage("ERROR: input argument required")
	}
	if len(Outputs) == 0 {
		ok = errorMessage("ERROR: output argument required")
	}
	if len(Configs) == 0 {
		ok = errorMessage("ERROR: number argument required")
	}
	if len(Configs) == 1 {
		Configs[0].Mode = Mode
		Configs[0].Alpha = Alpha
		Configs[0].Repeat = Repeat
		Configs[0].MaxQuadWidth = MaxQuadWidth
	}
	if !isValidAlpha(Alpha) {
		ok = errorMessage("ERROR: alpha must be from 0 to 255")
	}
	if !isValidMode(Mode) {
		ok = errorMessage("ERROR: mode must be from 0 to 8")
	}
	if !isValidQuadWidth(MaxQuadWidth) {
		ok = errorMessage("ERROR: curve width must be a finite number > 0")
	}
	for _, config := range Configs {
		if config.Count < 1 {
			ok = errorMessage("ERROR: number argument must be > 0")
		}
		if !isValidAlpha(config.Alpha) {
			ok = errorMessage("ERROR: alpha must be from 0 to 255")
		}
		if !isValidMode(config.Mode) {
			ok = errorMessage("ERROR: mode must be from 0 to 8")
		}
		if !isValidQuadWidth(config.MaxQuadWidth) {
			ok = errorMessage("ERROR: curve width must be a finite number > 0")
		}
	}
	if Candidates < 1 {
		ok = errorMessage("ERROR: candidate count must be > 0")
	}
	if Interval < 0 {
		ok = errorMessage("ERROR: negative frame interval not accepted")
	}
	if Interval > maxFrameInterval {
		ok = errorMessage(fmt.Sprintf("ERROR: frame interval must be at most %d", maxFrameInterval))
	}
	if Nth < 1 {
		ok = errorMessage("ERROR: frame step must be > 0")
	}
	if OutputSize < 1 {
		ok = errorMessage("ERROR: output size must be > 0")
	}
	for _, output := range Outputs {
		switch outputExtension(output) {
		case ".png", ".svg":
		case ".gif", ".jpg", ".jpeg":
			if OutputSize > maxOutputSide {
				ok = errorMessage(fmt.Sprintf("ERROR: output size of %s must be at most %d", output, maxOutputSide))
			}
		default:
			ok = errorMessage("ERROR: unrecognized file extension of " + output)
		}
		// A percent sign turns a path into a format string, and fmt marks a bad verb
		// in the result instead of failing, so the file gets that mark as its name.
		if strings.Contains(output, "%") && strings.Contains(fmt.Sprintf(output, 1), "%!") {
			ok = errorMessage("ERROR: " + output + " is not a valid frame pattern, write a literal percent sign as %%")
		}
	}
	var bg primitive.Color
	if Background != "" {
		var err error
		if bg, err = primitive.MakeHexColor(Background); err != nil {
			ok = errorMessage("ERROR: " + err.Error())
		}
	}
	if !ok {
		fmt.Println("Usage: primitive [OPTIONS] -i input -o output -n count")
		flag.PrintDefaults()
		os.Exit(1)
	}

	if V {
		primitive.LogLevel = 1
	}
	if VV {
		primitive.LogLevel = 2
	}

	if Seed == 0 {
		Seed = time.Now().UTC().UnixNano()
	}

	if Workers < 1 {
		Workers = runtime.NumCPU()
	}

	primitive.Log(1, "reading %s\n", Input)
	input, err := primitive.LoadImage(Input)
	check(err)

	if InputSize > 0 {
		input = makeThumbnail(input, InputSize)
	}

	if input.Bounds().Empty() {
		check(fmt.Errorf("input image %s has no pixels", Input))
	}

	if Background == "" {
		bg = primitive.MakeColor(primitive.AverageImageColor(input))
	}

	var startScore float64

	primitive.Log(1, "seed=%d\n", Seed)
	model := primitive.NewModel(input, bg, OutputSize, Workers, Seed)
	model.Candidates = Candidates
	primitive.Log(1, "%d: t=%.3f, score=%.6f\n", 0, 0.0, model.Score)

	startScore = model.Score

	start := time.Now()
	frame := 0
	for j, config := range Configs {
		primitive.Log(1, "count=%d, mode=%d, alpha=%d, repeat=%d\n",
			config.Count, config.Mode, config.Alpha, config.Repeat)

		for i := 0; i < config.Count; i++ {
			frame++

			t := time.Now()
			n := model.Step(primitive.ShapeType(config.Mode), config.Alpha, config.Repeat, config.MaxQuadWidth)
			nps := primitive.NumberString(float64(n) / time.Since(t).Seconds())
			elapsed := time.Since(start).Seconds()

			pctScore := 100 - (model.Score / startScore * 100)

			primitive.Log(1, "%d: t=%.3f, score=%.6f, n=%d, n/s=%s, percent score=%.3f%%\n", frame, elapsed, model.Score, n, nps, pctScore)

			for _, output := range Outputs {
				ext := outputExtension(output)
				percent := strings.Contains(output, "%")
				saveFrames := percent && ext != ".gif"
				saveFrames = saveFrames && frame%Nth == 0
				last := j == len(Configs)-1 && i == config.Count-1
				if saveFrames || last || model.Score <= Max || pctScore >= Maxpct {
					path := output
					if percent {
						path = fmt.Sprintf(output, frame)
					}
					primitive.Log(1, "writing %s\n", path)
					switch ext {
					case ".png":
						check(primitive.SavePNG(path, model.Context.Image()))
					case ".jpg", ".jpeg":
						check(primitive.SaveJPG(path, model.Context.Image(), 95))
					case ".svg":
						check(primitive.SaveFile(path, model.SVG()))
					case ".gif":
						frames := model.Frames(0.001)
						check(primitive.SaveGIF(path, frames, Interval, 250))
					}
				}
			}
			if model.Score <= Max || pctScore >= Maxpct {
				return
			}
		}
	}
}
