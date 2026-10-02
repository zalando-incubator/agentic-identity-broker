package main

import (
	"errors"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRunGate(t *testing.T) {
	tests := []struct {
		name            string
		change          func(t *testing.T, actual, baseline string)
		wantExit        int
		threshold       string
		changed         bool
		wantArtifacts   bool
		wantPartial     string
		missingArtifact string
		missingBaseline string
	}{
		{name: "required routes and manifest states pass", wantExit: 0, wantArtifacts: true},
		{
			name: "unreviewed state capture",
			change: func(t *testing.T, _, baseline string) {
				t.Helper()
				if err := os.Remove(filepath.Join(baseline, "state_light.png")); err != nil {
					t.Fatal(err)
				}
			},
			wantExit: 1, wantPartial: "state_light_actual.png", missingArtifact: "state_light_expected.png", missingBaseline: "state_light.png",
		},
		{
			name: "stale route baseline",
			change: func(t *testing.T, actual, _ string) {
				t.Helper()
				if err := os.Remove(filepath.Join(actual, "route_dark.png")); err != nil {
					t.Fatal(err)
				}
			},
			wantExit: 1, wantPartial: "route_dark_expected.png", missingArtifact: "route_dark_actual.png",
		},
		{
			name: "above threshold difference",
			change: func(t *testing.T, actual, _ string) {
				writeGatePNG(t, filepath.Join(actual, "route_light.png"), color.RGBA{B: 255, A: 255})
			},
			wantExit: 1, wantArtifacts: true, changed: true,
		},
		{
			name: "threshold boundary is inclusive",
			change: func(t *testing.T, actual, _ string) {
				writeGatePNG(t, filepath.Join(actual, "route_light.png"), color.RGBA{B: 255, A: 255})
			},
			threshold: "100", wantExit: 0, wantArtifacts: true, changed: true,
		},
		{
			name: "ungated documentation capture ignored",
			change: func(t *testing.T, actual, _ string) {
				writeGatePNG(t, filepath.Join(actual, "documentation_light.png"), color.RGBA{B: 255, A: 255})
			},
			wantExit: 0, wantArtifacts: true,
		},
		{
			name: "invalid actual image",
			change: func(t *testing.T, actual, _ string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(actual, "state_dark.png"), []byte("not a PNG"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantExit: 1,
		},
		{
			name: "invalid reviewed baseline",
			change: func(t *testing.T, _, baseline string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(baseline, "route_light.png"), []byte("not a PNG"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, baseline, manifest, out := gateFixture(t)
			if tt.change != nil {
				tt.change(t, actual, baseline)
			}
			args := gateArgs(actual, baseline, manifest, out)
			if tt.threshold != "" {
				args[9] = tt.threshold
			}
			if got := runGate(args); got != tt.wantExit {
				t.Fatalf("runGate exit = %d, want %d", got, tt.wantExit)
			}
			if tt.wantArtifacts {
				assertGateArtifacts(t, out, tt.changed)
			}
			if tt.wantPartial != "" {
				if _, err := os.Stat(filepath.Join(out, tt.wantPartial)); err != nil {
					t.Errorf("available capture/baseline artifact missing: %v", err)
				}
				if _, err := os.Stat(filepath.Join(out, tt.missingArtifact)); !os.IsNotExist(err) {
					t.Errorf("missing image represented as artifact: %v", err)
				}
			}
			if tt.missingBaseline != "" {
				if _, err := os.Stat(filepath.Join(baseline, tt.missingBaseline)); !os.IsNotExist(err) {
					t.Errorf("unreviewed image was accepted as baseline: %v", err)
				}
			}
		})
	}
}

func TestRunGateRejectsInvalidSetup(t *testing.T) {
	tests := []struct {
		name   string
		change func(t *testing.T, args []string, manifest string)
	}{
		{
			name:   "required stem traverses parent",
			change: func(_ *testing.T, args []string, _ string) { args[5] = "../route" },
		},
		{
			name:   "required stem traverses windows path",
			change: func(_ *testing.T, args []string, _ string) { args[5] = `..\route` },
		},
		{
			name: "manifest stem traverses directory",
			change: func(t *testing.T, _ []string, manifest string) {
				t.Helper()
				if err := os.WriteFile(manifest, []byte("# states\n../state\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "missing mandatory manifest",
			change: func(_ *testing.T, args []string, _ string) { args[7] += ".missing" },
		},
		{
			name:   "unreadable manifest directory",
			change: func(_ *testing.T, args []string, _ string) { args[7] = filepath.Dir(args[7]) },
		},
		{
			name:   "not-a-number threshold",
			change: func(_ *testing.T, args []string, _ string) { args[9] = "NaN" },
		},
		{
			name:   "infinite threshold",
			change: func(_ *testing.T, args []string, _ string) { args[9] = "+Inf" },
		},
		{
			name:   "negative threshold",
			change: func(_ *testing.T, args []string, _ string) { args[9] = "-0.1" },
		},
		{
			name:   "threshold above 100",
			change: func(_ *testing.T, args []string, _ string) { args[9] = "100.1" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, baseline, manifest, out := gateFixture(t)
			args := gateArgs(actual, baseline, manifest, out)
			tt.change(t, args, manifest)
			if got := runGate(args); got != 2 {
				t.Fatalf("runGate exit = %d, want setup error 2", got)
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatalf("output directory created on invalid setup: stat err = %v", err)
			}
		})
	}
}

func gateFixture(t *testing.T) (actual, baseline, manifest, out string) {
	t.Helper()
	root := t.TempDir()
	actual = filepath.Join(root, "actual")
	baseline = filepath.Join(root, "reviewed")
	manifest = filepath.Join(root, "states.txt")
	out = filepath.Join(root, "diff")
	for _, dir := range []string{actual, baseline} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(manifest, []byte("# reviewed states\n\nstate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, stem := range []string{"route", "route_two", "state"} {
		for _, theme := range []string{"light", "dark"} {
			filename := stem + "_" + theme + ".png"
			for _, dir := range []string{actual, baseline} {
				writeGatePNG(t, filepath.Join(dir, filename), color.RGBA{R: 255, A: 255})
			}
		}
	}
	return actual, baseline, manifest, out
}

func gateArgs(actual, baseline, manifest, out string) []string {
	return []string{"--actual", actual, "--baseline", baseline, "--required", "route,route_two", "--manifest", manifest, "--threshold", "0.5", "--out", out}
}

func writeGatePNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, solidImage(2, 2, c)); err != nil {
		t.Fatal(errors.Join(err, file.Close()))
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertGateArtifacts(t *testing.T, out string, changed bool) {
	t.Helper()
	var want []string
	for _, stem := range []string{"route", "route_two", "state"} {
		for _, theme := range []string{"light", "dark"} {
			for _, kind := range []string{"actual", "diff", "expected"} {
				want = append(want, stem+"_"+theme+"_"+kind+".png")
			}
		}
	}
	sort.Strings(want)
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
		file, err := os.Open(filepath.Join(out, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(file)
		err = errors.Join(err, file.Close())
		if err != nil {
			t.Fatalf("%s is not a valid PNG: %v", entry.Name(), err)
		}
		pixel := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
		if strings.HasSuffix(entry.Name(), "_diff.png") {
			wantPixel := color.NRGBA{}
			if entry.Name() == "route_light_diff.png" && changed {
				wantPixel = color.NRGBA{R: 255, A: 255}
			}
			if pixel != wantPixel {
				t.Errorf("diff pixel in %s = %v, want %v", entry.Name(), pixel, wantPixel)
			}
		} else {
			wantPixel := color.NRGBA{R: 255, A: 255}
			if entry.Name() == "route_light_actual.png" && changed {
				wantPixel = color.NRGBA{B: 255, A: 255}
			}
			if pixel != wantPixel {
				t.Errorf("artifact pixel in %s = %v, want %v", entry.Name(), pixel, wantPixel)
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("artifacts = %v, want %v", got, want)
	}
}
