// SPDX-License-Identifier: AGPL-3.0-only
package lint

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// wantLines are the fixture lines marked "// want".
func wantLines(t *testing.T, path string) []int {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var lines []int
	scanner := bufio.NewScanner(file)
	for n := 1; scanner.Scan(); n++ {
		if strings.HasSuffix(strings.TrimSpace(scanner.Text()), "// want") {
			lines = append(lines, n)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

// RUN-7: the rule reports exactly the errors given to _, through the real
// loader, and nothing typed as a bool, an int or a documented-infallible
// write.
func TestDiscardedErrorsFixture(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := Check(root, "./internal/lint/testdata/discard")
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, f := range findings {
		if filepath.Base(f.Pos.Filename) != "discard.go" {
			t.Fatalf("finding outside the fixture: %s", f)
		}
		got = append(got, f.Pos.Line)
	}
	want := wantLines(t, filepath.Join("testdata", "discard", "discard.go"))
	if !slices.Equal(got, want) {
		t.Fatalf("reported lines %v, want %v:\n%v", got, want, findings)
	}
	if !strings.Contains(findings[0].String(), "error from fails discarded") {
		t.Fatalf("message = %q", findings[0])
	}
}

func TestRunFailsOnFindings(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(&out, root, "./internal/lint/testdata/discard"); !errors.Is(err, ErrFindings) {
		t.Fatalf("err=%v, want ErrFindings", err)
	}
	if strings.Count(out.String(), "\n") != len(wantLines(t, filepath.Join("testdata", "discard", "discard.go"))) {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	if err := Run(&out, root, "./internal/observe"); err != nil || out.Len() != 0 {
		t.Fatalf("clean package: err=%v output=%q", err, out.String())
	}
}
