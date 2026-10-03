package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// runGate compares only required route and reviewed state captures. It never
// copies a capture into the baseline directory.
func runGate(args []string) int {
	flags := flag.NewFlagSet("gate", flag.ContinueOnError)
	actualDir := flags.String("actual", "", "directory of captured PNGs")
	baselineDir := flags.String("baseline", "", "directory of reviewed PNGs")
	required := flags.String("required", "", "comma-separated route stems")
	manifest := flags.String("manifest", "", "file of reviewed state stems")
	thresholdArg := flags.String("threshold", "0.5", "maximum percentage of differing pixels")
	outDir := flags.String("out", "", "directory for comparison artifacts")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *actualDir == "" || *baselineDir == "" || *required == "" || *manifest == "" || *outDir == "" {
		fmt.Fprintln(os.Stderr, "gate requires --actual, --baseline, --required, --manifest and --out")
		return 2
	}
	threshold, err := strconv.ParseFloat(*thresholdArg, 64)
	if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		fmt.Fprintln(os.Stderr, "gate threshold must be a finite percentage from 0 to 100")
		return 2
	}
	stems, err := gateStems(*required, *manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	for _, dir := range []string{*actualDir, *baselineDir} {
		info, err := os.Stat(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gate directory %s: %v\n", dir, err)
			return 2
		}
		if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "gate path %s is not a directory\n", dir)
			return 2
		}
	}
	if err := os.MkdirAll(*outDir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "gate output directory %s: %v\n", *outDir, err)
		return 2
	}

	failed := false
	for _, stem := range stems {
		for _, theme := range []string{"light", "dark"} {
			name := stem + "_" + theme
			baselinePath := filepath.Join(*baselineDir, name+".png")
			actualPath := filepath.Join(*actualDir, name+".png")
			expectedImage, baselineErr := loadPNG(baselinePath)
			actualImage, actualErr := loadPNG(actualPath)
			if baselineErr != nil {
				fmt.Fprintf(os.Stderr, "%s: missing or invalid reviewed baseline %s: %v\n", name, baselinePath, baselineErr)
				failed = true
			}
			if actualErr != nil {
				fmt.Fprintf(os.Stderr, "%s: missing or invalid capture %s: %v\n", name, actualPath, actualErr)
				failed = true
			}
			var percent float64
			var diff image.Image
			if baselineErr == nil && actualErr == nil {
				expected := toNRGBA(expectedImage)
				actual := toNRGBA(actualImage)
				percent = diffPercent(expected, actual)
				fmt.Printf("%s: %.4f%%\n", name, percent)
				diff = gateDiff(expected, actual)
			}
			for _, artifact := range []struct {
				kind string
				img  image.Image
				err  error
			}{
				{"expected", expectedImage, baselineErr},
				{"actual", actualImage, actualErr},
				{"diff", diff, nil},
			} {
				if artifact.err != nil || artifact.img == nil {
					continue
				}
				path := filepath.Join(*outDir, name+"_"+artifact.kind+".png")
				if err := saveGatePNG(path, artifact.img); err != nil {
					fmt.Fprintf(os.Stderr, "gate artifact %s: %v\n", path, err)
					return 2
				}
			}
			if baselineErr == nil && actualErr == nil && percent > threshold {
				fmt.Fprintf(os.Stderr, "%s: %.4f%% exceeds %.4f%% threshold\n", name, percent, threshold)
				failed = true
			}
		}
	}
	if failed {
		return 1
	}
	return 0
}

func gateStems(required, manifestPath string) ([]string, error) {
	stems := make(map[string]struct{})
	for _, part := range strings.Split(required, ",") {
		stem := strings.TrimSpace(part)
		if !validGateStem(stem) {
			return nil, fmt.Errorf("invalid required stem %q", stem)
		}
		stems[stem] = struct{}{}
	}
	file, err := os.OpenInRoot(filepath.Dir(manifestPath), filepath.Base(manifestPath))
	if err != nil {
		return nil, fmt.Errorf("manifest %s: %w", manifestPath, err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		stem := strings.TrimSpace(scanner.Text())
		if stem == "" || strings.HasPrefix(stem, "#") {
			continue
		}
		if !validGateStem(stem) {
			return nil, errors.Join(fmt.Errorf("manifest %s: invalid state stem %q", manifestPath, stem), file.Close())
		}
		stems[stem] = struct{}{}
	}
	scanErr := scanner.Err()
	closeErr := file.Close()
	if scanErr != nil {
		return nil, fmt.Errorf("manifest %s: %w", manifestPath, scanErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("manifest %s: %w", manifestPath, closeErr)
	}
	ordered := make([]string, 0, len(stems))
	for stem := range stems {
		ordered = append(ordered, stem)
	}
	sort.Strings(ordered)
	return ordered, nil
}

func validGateStem(stem string) bool {
	if stem == "" || stem == "." || stem == ".." {
		return false
	}
	for _, r := range stem {
		allowed := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !allowed {
			return false
		}
	}
	return true
}

func gateDiff(expected, actual *image.NRGBA) *image.NRGBA {
	a, b := expected.Bounds(), actual.Bounds()
	width, height := max(a.Dx(), b.Dx()), max(a.Dy(), b.Dy())
	result := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			point := image.Pt(x, y)
			if !point.In(a) || !point.In(b) || expected.NRGBAAt(x, y) != actual.NRGBAAt(x, y) {
				result.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			}
		}
	}
	return result
}

func saveGatePNG(path string, img image.Image) error {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file, err := root.OpenFile(filepath.Base(path), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := png.Encode(file, img); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}
