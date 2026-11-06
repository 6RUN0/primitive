package main

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/6RUN0/primitive/primitive"
)

const runMainVariable = "PRIMITIVE_TEST_RUN_MAIN"

// TestMain turns the test binary into the CLI for child processes: main ends in
// log.Fatal or os.Exit, and only a real process shows the exit code.
func TestMain(m *testing.M) {
	if os.Getenv(runMainVariable) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliResult struct {
	stdout, stderr string
	exitCode       int
}

func runCLI(t *testing.T, arguments ...string) cliResult {
	t.Helper()
	command := exec.Command(os.Args[0], arguments...)
	command.Env = append(os.Environ(), runMainVariable+"=1")
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if _, isExit := err.(*exec.ExitError); err != nil && !isExit {
		t.Fatal(err)
	}
	return cliResult{stdout.String(), stderr.String(), command.ProcessState.ExitCode()}
}

func writeTestPNG(t *testing.T, width, height int) string {
	t.Helper()
	picture := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / width), G: uint8(y * 255 / height), B: uint8((x ^ y) * 16), A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "input.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
	return path
}

func decodeImageSize(t *testing.T, path string) image.Point {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	return image.Pt(config.Width, config.Height)
}

// TestCLIRejectsInvalidArguments pins argument validation: each value ends with
// exit code 1 and a message naming the problem, not with a panic later on or
// with a silently wrong background color.
func TestCLIRejectsInvalidArguments(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	output := filepath.Join(t.TempDir(), "output.png")
	cases := []struct {
		name      string
		arguments []string
		message   string
	}{
		{"no arguments", nil, "input argument required"},
		{"no output", []string{"-i", input, "-n", "1"}, "output argument required"},
		{"no count", []string{"-i", input, "-o", output}, "number argument required"},
		{"zero count", []string{"-i", input, "-o", output, "-n", "0"}, "number argument must be > 0"},
		{"count is not a number", []string{"-i", input, "-o", output, "-n", "many"}, "number argument must be > 0"},
		{"zero candidates", []string{"-i", input, "-o", output, "-n", "1", "-nc", "0"}, "candidate count must be > 0"},
		{"negative candidates", []string{"-i", input, "-o", output, "-n", "1", "-nc", "-1"}, "candidate count must be > 0"},
		{"negative interval", []string{"-i", input, "-o", output, "-n", "1", "-int", "-1"}, "negative frame interval"},
		{"interval above the GIF limit", []string{"-i", input, "-o", output, "-n", "1", "-int", "65536"}, "frame interval must be at most 65535"},
		{"GIF size above the limit", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "large.gif"), "-n", "1", "-s", "65536"}, "must be at most 65535"},
		{"JPEG size above the limit", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "large.JPG"), "-n", "1", "-s", "65536"}, "must be at most 65535"},
		{"zero output size", []string{"-i", input, "-o", output, "-n", "1", "-s", "0"}, "output size must be > 0"},
		{"negative output size", []string{"-i", input, "-o", output, "-n", "1", "-s", "-8"}, "output size must be > 0"},
		{"alpha above range", []string{"-i", input, "-o", output, "-n", "1", "-a", "300"}, "alpha must be from 0 to 255"},
		{"negative alpha", []string{"-i", input, "-o", output, "-n", "1", "-a", "-5"}, "alpha must be from 0 to 255"},
		{"alpha of an earlier config", []string{"-i", input, "-o", output, "-a", "256", "-n", "1", "-a", "1", "-n", "1"}, "alpha must be from 0 to 255"},
		{"prefixed background", []string{"-i", input, "-o", output, "-n", "1", "-bg", "0X00"}, `invalid hex color "0X00"`},
		{"short background", []string{"-i", input, "-o", output, "-n", "1", "-bg", "ff"}, `invalid hex color "ff"`},
		{"missing input file", []string{"-i", input + ".absent", "-o", output, "-n", "1"}, "no such file"},
		{"zero frame step", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "a%d.png"), "-n", "1", "-nth", "0"}, "frame step must be > 0"},
		{"negative frame step", []string{"-i", input, "-o", output, "-n", "1", "-nth", "-1"}, "frame step must be > 0"},
		{"negative curve width", []string{"-i", input, "-o", output, "-n", "1", "-mqw", "-1"}, "curve width must be a finite number > 0"},
		{"zero curve width", []string{"-i", input, "-o", output, "-n", "1", "-mqw", "0"}, "curve width must be a finite number > 0"},
		{"curve width is not a number", []string{"-i", input, "-o", output, "-n", "1", "-mqw", "NaN"}, "curve width must be a finite number > 0"},
		{"infinite curve width", []string{"-i", input, "-o", output, "-n", "1", "-mqw", "Inf"}, "curve width must be a finite number > 0"},
		{"curve width of an earlier group", []string{"-i", input, "-o", output, "-mqw", "-1", "-n", "1", "-mqw", "1", "-n", "1"}, "curve width must be a finite number > 0"},
		{"mode above the range", []string{"-i", input, "-o", output, "-n", "1", "-m", "9"}, "mode must be from 0 to 8"},
		{"negative mode", []string{"-i", input, "-o", output, "-n", "1", "-m", "-1"}, "mode must be from 0 to 8"},
		{"mode of an earlier group", []string{"-i", input, "-o", output, "-m", "9", "-n", "1", "-m", "1", "-n", "1"}, "mode must be from 0 to 8"},
		{"percent sign without a verb", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "a%.png"), "-n", "1"}, "not a valid frame pattern"},
		{"pattern with two verbs", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "a%d%d.png"), "-n", "1"}, "not a valid frame pattern"},
		{"no output type", []string{"-i", input, "-o", filepath.Join(filepath.Dir(output), "noextension"), "-n", "1"}, "unrecognized file extension"},
		{"unknown output type", []string{"-i", input, "-o", output + ".bmp", "-n", "1"}, "unrecognized file extension"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runCLI(t, testCase.arguments...)
			if result.exitCode != 1 {
				t.Errorf("exit code %d, want 1", result.exitCode)
			}
			if !strings.Contains(result.stderr, testCase.message) {
				t.Errorf("stderr %q, want %q in it", result.stderr, testCase.message)
			}
			if strings.Contains(result.stderr, "goroutine 1 [") {
				t.Errorf("panic in stderr: %s", result.stderr)
			}
		})
	}
}

// TestCLIAcceptsBoundaryValues pins the edges of the accepted ranges: each value
// passes argument validation and the run goes on to read the input, which is
// absent, so that a check rejecting an edge value shows as a usage error.
func TestCLIAcceptsBoundaryValues(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.png")
	output := filepath.Join(t.TempDir(), "output.png")
	gifOutput := filepath.Join(filepath.Dir(output), "output.gif")
	cases := []struct {
		name      string
		arguments []string
	}{
		{"alpha of zero", []string{"-o", output, "-a", "0"}},
		{"alpha of 255", []string{"-o", output, "-a", "255"}},
		{"mode of zero", []string{"-o", output, "-m", "0"}},
		{"mode of eight", []string{"-o", output, "-m", "8"}},
		{"interval of zero", []string{"-o", output, "-int", "0"}},
		{"interval at the GIF limit", []string{"-o", output, "-int", "65535"}},
		{"output size of one", []string{"-o", output, "-s", "1"}},
		{"GIF size at the limit", []string{"-o", gifOutput, "-s", "65535"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := runCLI(t, append([]string{"-i", missing, "-n", "1"}, testCase.arguments...)...)
			if !strings.Contains(result.stderr, "no such file") || strings.Contains(result.stderr, "ERROR:") {
				t.Errorf("stderr %q, want the missing input and no usage error", result.stderr)
			}
		})
	}
}

// TestCLIOneWorkerIsNotAllCores pins -j 1 end to end: a run with one worker must
// not be widened to every core, which gives another picture for one seed.
func TestCLIOneWorkerIsNotAllCores(t *testing.T) {
	if runtime.NumCPU() < 2 {
		t.Skip("one core: -j 1 and the default are the same run")
	}
	input := writeTestPNG(t, 24, 24)
	render := func(extra ...string) string {
		result := runCLI(t, append([]string{"-i", input, "-o", "-", "-n", "3", "-r", "24", "-s", "48", "-seed", "5"}, extra...)...)
		if result.exitCode != 0 {
			t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
		}
		return result.stdout
	}
	if render("-j", "1") == render() {
		t.Error("-j 1 and the default gave one picture")
	}
}

// TestCLISeedRepeatsOutput pins -seed end to end for one and several workers,
// and that the default 0 draws a new seed for each run.
func TestCLISeedRepeatsOutput(t *testing.T) {
	input := writeTestPNG(t, 24, 24)
	render := func(extra ...string) string {
		arguments := append([]string{"-i", input, "-o", "-", "-n", "3", "-r", "24", "-s", "48"}, extra...)
		result := runCLI(t, arguments...)
		if result.exitCode != 0 {
			t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
		}
		return result.stdout
	}
	for _, workers := range []string{"1", "4"} {
		first, second := render("-j", workers, "-seed", "5"), render("-j", workers, "-seed", "5")
		if first != second {
			t.Errorf("-j %s: two runs with one seed differ", workers)
		}
	}
	first, second := render("-j", "1"), render("-j", "1")
	if first == second {
		t.Error("two runs without a seed gave one output")
	}
}

// TestCLICandidatesReachSearch pins -nc end to end: a value that stops at the flag
// leaves the default in the search, and both runs give one picture.
func TestCLICandidatesReachSearch(t *testing.T) {
	input := writeTestPNG(t, 24, 24)
	render := func(candidates string) string {
		result := runCLI(t, "-i", input, "-o", "-", "-n", "4", "-r", "24", "-s", "48", "-j", "1", "-seed", "5", "-nc", candidates)
		if result.exitCode != 0 {
			t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
		}
		return result.stdout
	}
	if render("1") == render(strconv.Itoa(primitive.DefaultCandidates)) {
		t.Error("-nc 1 and the default gave one picture")
	}
}

// TestCLIOutputBelowInputSize pins -s smaller than the resized input: a scale
// below 1 must not reach an integer division in NewModel.
func TestCLIOutputBelowInputSize(t *testing.T) {
	input := writeTestPNG(t, 64, 32)
	output := filepath.Join(t.TempDir(), "output.png")
	result := runCLI(t, "-i", input, "-o", output, "-n", "1", "-s", "32", "-seed", "1")
	if result.exitCode != 0 {
		t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
	}
	if got, want := decodeImageSize(t, output), image.Pt(32, 16); got != want {
		t.Errorf("output size %v, want %v", got, want)
	}
}

// TestCLIAcceptsOffsetGIF pins a GIF whose first frame does not start at the
// origin and fits the -r limit: it reaches the search unscaled, with a non-zero
// Min, and has to be moved to the origin.
func TestCLIAcceptsOffsetGIF(t *testing.T) {
	frame := image.NewPaletted(image.Rect(20, 20, 70, 70), color.Palette{color.Black, color.White})
	for i := range frame.Pix {
		frame.Pix[i] = uint8(i % 2)
	}
	var encoded bytes.Buffer
	err := gif.EncodeAll(&encoded, &gif.GIF{
		Image:  []*image.Paletted{frame},
		Delay:  []int{0},
		Config: image.Config{ColorModel: frame.Palette, Width: 100, Height: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "offset.gif")
	if err := os.WriteFile(input, encoded.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "output.png")
	if result := runCLI(t, "-i", input, "-o", output, "-n", "1", "-s", "100", "-seed", "1"); result.exitCode != 0 {
		t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
	}
	if got, want := decodeImageSize(t, output), image.Pt(100, 100); got != want {
		t.Errorf("output size %v, want %v", got, want)
	}
}

// TestCLIRejectsInputWithoutPixels pins a decodable file of zero size: NewModel
// cannot work on it, so the CLI stops with a message.
func TestCLIRejectsInputWithoutPixels(t *testing.T) {
	// 0x0 GIF89a: screen and frame of zero size, LZW stream with clear and end codes.
	contents := []byte("GIF89a\x00\x00\x00\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff,\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x01\x2c\x00;")
	directory := t.TempDir()
	input := filepath.Join(directory, "empty.gif")
	if err := os.WriteFile(input, contents, 0o644); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, "-i", input, "-o", filepath.Join(directory, "output.png"), "-n", "1")
	if result.exitCode != 1 || !strings.Contains(result.stderr, "has no pixels") {
		t.Errorf("exit code %d, stderr %q", result.exitCode, result.stderr)
	}
}

// TestCLIConfigOrder pins the order rule of repeated -n: each count takes the
// -m value parsed before it, and the last -m does not rewrite earlier counts.
func TestCLIConfigOrder(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	result := runCLI(t, "-i", input, "-o", "-", "-j", "1", "-seed", "1", "-m", "3", "-n", "2", "-m", "1", "-n", "1", "-s", "32")
	if result.exitCode != 0 {
		t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
	}
	if got := strings.Count(result.stdout, "<ellipse "); got != 2 {
		t.Errorf("got %d ellipses, want 2", got)
	}
	if got := strings.Count(result.stdout, "<path fill="); got != 1 {
		t.Errorf("got %d triangles, want 1", got)
	}
}

// TestCLINthFrame pins -nth with a %d path: frames are numbered from 1 and only
// every Nth one is written.
func TestCLINthFrame(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	directory := t.TempDir()
	pattern := filepath.Join(directory, "frame%d.svg")
	result := runCLI(t, "-i", input, "-o", pattern, "-j", "1", "-seed", "1", "-n", "4", "-nth", "2", "-s", "32")
	if result.exitCode != 0 {
		t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"frame2.svg", "frame4.svg"}; len(files) != 2 || filepath.Base(files[0]) != want[0] || filepath.Base(files[1]) != want[1] {
		t.Errorf("got files %v, want %v", files, want)
	}
}

// TestCLIStopsOnExactMatch pins -ma and -mp on flat graphics, where opaque rectangles
// reach an exact match: a score that wraps uint64 at that point jumps to millions,
// so neither limit fires and the run goes on spoiling the picture.
func TestCLIStopsOnExactMatch(t *testing.T) {
	picture := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			fill := color.NRGBA{R: 249, G: 14, B: 199, A: 255}
			if x >= 2 && y >= 2 {
				fill = color.NRGBA{R: 221, G: 1, B: 228, A: 255}
			}
			picture.SetNRGBA(x, y, fill)
		}
	}
	input := filepath.Join(t.TempDir(), "flat.png")
	output := filepath.Join(t.TempDir(), "output.svg")
	file, err := os.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
	for _, limit := range [][]string{{"-ma", "0.000001"}, {"-mp", "99.999"}} {
		for seed := 1; seed <= 3; seed++ {
			arguments := []string{"-i", input, "-o", output, "-bg", "654321", "-m", "2", "-a", "255", "-n", "40", "-j", "1", "-s", "8", "-seed", strconv.Itoa(seed), "-v"}
			result := runCLI(t, append(arguments, limit...)...)
			if result.exitCode != 0 {
				t.Fatalf("exit code %d: %s", result.exitCode, result.stderr)
			}
			if strings.Contains(result.stdout, "\n40: ") {
				t.Errorf("%v, seed %d: the run did not stop", limit, seed)
			}
		}
	}
}

// TestCLIStopsAtTargetScore pins -ma and -mp: the run ends at the first shape
// that reaches the target, with the output written.
func TestCLIStopsAtTargetScore(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	for _, target := range [][]string{{"-ma", "1"}, {"-mp", "-1000"}} {
		arguments := append([]string{"-i", input, "-o", "-", "-j", "1", "-seed", "1", "-n", "5", "-s", "32"}, target...)
		result := runCLI(t, arguments...)
		if result.exitCode != 0 {
			t.Fatalf("%v: exit code %d: %s", target, result.exitCode, result.stderr)
		}
		if got := strings.Count(result.stdout, "<path fill="); got != 1 {
			t.Errorf("%v: got %d shapes, want 1", target, got)
		}
	}
}

// TestShapeConfigSetCapturesFlags pins that a parsed -n stores the flag values
// current at that point of the command line.
func TestShapeConfigSetCapturesFlags(t *testing.T) {
	Mode, Alpha, Repeat, MaxQuadWidth = 6, 200, 3, 2.5
	t.Cleanup(func() { Mode, Alpha, Repeat, MaxQuadWidth = 1, 128, 0, 0.5 })
	var configs shapeConfigArray
	if err := configs.Set("0x10"); err != nil {
		t.Fatal(err)
	}
	want := shapeConfig{Count: 16, Mode: 6, Alpha: 200, Repeat: 3, MaxQuadWidth: 2.5}
	if len(configs) != 1 || configs[0] != want {
		t.Errorf("got %+v, want %+v", configs, want)
	}
}

// TestOutputsKeepOrder pins that -o accumulates instead of replacing.
func TestOutputsKeepOrder(t *testing.T) {
	var outputs flagArray
	for _, path := range []string{"a.png", "b.svg"} {
		if err := outputs.Set(path); err != nil {
			t.Fatal(err)
		}
	}
	if got := outputs.String(); got != "a.png, b.svg" {
		t.Errorf("got %q", got)
	}
}

// TestCLIWritesAnimatedGIF pins .gif output end to end: the -int delay on the
// frames, 250 on the last one, and no external program in the path.
func TestCLIWritesAnimatedGIF(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	output := filepath.Join(t.TempDir(), "output.gif")
	command := exec.Command(os.Args[0], "-i", input, "-o", output, "-n", "4", "-s", "32", "-j", "1", "-seed", "1", "-int", "7")
	command.Env = []string{runMainVariable + "=1", "PATH="}
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, combined)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	animation, err := gif.DecodeAll(file)
	if err != nil {
		t.Fatal(err)
	}
	lastFrame := len(animation.Image) - 1
	if lastFrame < 1 {
		t.Fatalf("got %d frames, want an animation", lastFrame+1)
	}
	if animation.Delay[0] != 7 || animation.Delay[lastFrame] != 250 {
		t.Errorf("delays %v, want 7 and a last delay of 250", animation.Delay)
	}
}

// TestCLIReportsWriteError pins the exit code: a run that lost its result must
// not look successful to a script.
func TestCLIReportsWriteError(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	directory := t.TempDir()
	for _, name := range []string{"output.png", "output.jpg", "output.svg", "output.gif"} {
		output := filepath.Join(directory, "absent", name)
		result := runCLI(t, "-i", input, "-o", output, "-n", "1", "-s", "16")
		if result.exitCode != 1 {
			t.Errorf("%s: exit code %d, want 1", name, result.exitCode)
		}
		if !strings.Contains(result.stderr, "no such file") {
			t.Errorf("%s: stderr %q", name, result.stderr)
		}
	}
}

// TestCLIChecksOutputsBeforeSearch pins the order of the checks: an unknown type
// surfaced at the last shape, after the whole search and the outputs before it.
func TestCLIChecksOutputsBeforeSearch(t *testing.T) {
	input := writeTestPNG(t, 16, 16)
	directory := t.TempDir()
	valid := filepath.Join(directory, "output.png")
	result := runCLI(t, "-i", input, "-o", valid, "-o", filepath.Join(directory, "output.bmp"), "-n", "1", "-s", "16")
	if result.exitCode != 1 {
		t.Errorf("exit code %d, want 1", result.exitCode)
	}
	if _, err := os.Stat(valid); !os.IsNotExist(err) {
		t.Errorf("the search ran and wrote %s: %v", valid, err)
	}
}

// TestCLIPrintsDefaultVersion pins the value of a build without ldflags, and that
// -version needs no other argument.
func TestCLIPrintsDefaultVersion(t *testing.T) {
	result := runCLI(t, "-version")
	if result.exitCode != 0 || result.stdout != "devel\n" {
		t.Errorf("exit code %d, stdout %q, want 0 and devel", result.exitCode, result.stdout)
	}
}

// TestVersionComesFromLinker pins the variable name that .goreleaser.yaml passes
// to -X: the linker ignores a name that does not exist, without an error.
func TestVersionComesFromLinker(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "primitive.bin")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=1.2.3", "-o", binary, ".")
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, combined)
	}
	printed, err := exec.Command(binary, "-version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(printed) != "1.2.3\n" {
		t.Errorf("got %q", printed)
	}
}
