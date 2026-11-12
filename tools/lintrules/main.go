// Command lintrules proves that the lint configuration rejects a deliberate violation of each
// rule. It runs golangci-lint with the repository's .golangci.yml on the fixture packages under
// tools/lintrules/testdata and compares the reported issues with the fixtures' want comments:
//
//	call() // want <linter> `<regexp>` [<linter> `<regexp>` ...]
//
// Each want must match an issue of that linter on the same line, and each issue must match a
// want. Run it from the repository root:
//
//	lintrules [-bin ./bin/golangci-lint-custom] [-dir tools/lintrules/testdata]
//
// Exit codes: 0 when the issues match the wants, 1 when they differ, 2 on a usage or run error.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type want struct {
	file   string
	line   int
	linter string
	text   *regexp.Regexp
	found  bool
}

type issue struct {
	FromLinter string
	Text       string
	Pos        struct {
		Filename string
		Line     int
	}
}

var (
	wantComment = regexp.MustCompile(`// want (.*)$`)
	wantPair    = regexp.MustCompile("([a-z]+) `([^`]*)`")
)

func main() {
	bin := flag.String("bin", "./bin/golangci-lint-custom", "golangci-lint binary")
	dir := flag.String("dir", "tools/lintrules/testdata", "fixture root")
	flag.Parse()

	wants, packages, err := readWants(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lintrules:", err)
		os.Exit(2)
	}
	issues, err := runLint(*bin, packages)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lintrules:", err)
		os.Exit(2)
	}

	var failures []string
	for _, is := range issues {
		file := filepath.ToSlash(filepath.Clean(is.Pos.Filename))
		matched := false
		for _, w := range wants {
			if w.file == file && w.line == is.Pos.Line && w.linter == is.FromLinter && w.text.MatchString(is.Text) {
				w.found, matched = true, true
			}
		}
		if !matched {
			failures = append(failures, fmt.Sprintf("%s:%d: unexpected %s issue: %s", file, is.Pos.Line, is.FromLinter, is.Text))
		}
	}
	for _, w := range wants {
		if !w.found {
			failures = append(failures, fmt.Sprintf("%s:%d: no %s issue matches `%s`", w.file, w.line, w.linter, w.text))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		os.Exit(1)
	}
	fmt.Printf("lintrules: %d deliberate violations rejected as expected\n", len(wants))
}

// readWants returns the want comments and the package directories under dir. The go command
// skips testdata directories in ./... patterns, so each package is passed by its directory.
func readWants(dir string) ([]*want, []string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()

	var wants []*want
	packages := map[string]bool{}
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file := filepath.ToSlash(filepath.Join(dir, path))
		packages["./"+filepath.ToSlash(filepath.Dir(file))] = true
		src, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(bytes.NewReader(src))
		for line := 1; scanner.Scan(); line++ {
			m := wantComment.FindStringSubmatch(scanner.Text())
			if m == nil {
				continue
			}
			pairs := wantPair.FindAllStringSubmatch(m[1], -1)
			if len(pairs) == 0 {
				return fmt.Errorf("%s:%d: malformed want comment", file, line)
			}
			for _, p := range pairs {
				re, err := regexp.Compile(p[2])
				if err != nil {
					return fmt.Errorf("%s:%d: %w", file, line, err)
				}
				wants = append(wants, &want{file: file, line: line, linter: p[1], text: re})
			}
		}
		return scanner.Err()
	})
	if err == nil && len(wants) == 0 {
		err = fmt.Errorf("no want comments under %s", dir)
	}
	dirs := make([]string, 0, len(packages))
	for p := range packages {
		dirs = append(dirs, p)
	}
	sort.Strings(dirs)
	return wants, dirs, err
}

func runLint(bin string, packages []string) ([]issue, error) {
	args := append([]string{"run", "--output.json.path=stdout", "--output.text.path=", "--show-stats=false"}, packages...)
	cmd := exec.Command(bin, args...) // #nosec G204 -- runs the golangci-lint binary that the developer passes with -bin.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		return nil, fmt.Errorf("%s: %w\n%s", bin, err, stderr.String())
	}
	var report struct{ Issues []issue }
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return nil, fmt.Errorf("decode %s output: %w\n%s", bin, err, stderr.String())
	}
	return report.Issues, nil
}
