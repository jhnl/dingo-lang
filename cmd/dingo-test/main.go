package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jhnl/dingo/backend"

	"github.com/jhnl/dingo/semantics"

	"github.com/jhnl/dingo/common"

	"github.com/jhnl/dingo/ir"
	"github.com/jhnl/dingo/token"

	"github.com/jhnl/dingo/module"
)

func main() {
	var manifest string

	flag.StringVar(&manifest, "manifest", "", "Test manifest")
	flag.Parse()

	var tests []*testCase
	tester := &testRunner{}

	if len(manifest) > 0 {
		tests = readTestManifest(manifest)
		tester.baseDir = filepath.Dir(manifest)
	} else {
		tests = createTests(flag.Args())
	}

	tester.runTests("", tests)
	fmt.Printf("\nFINISHED: total: %d success: %d fail: %d skip: %d bad: %d\n\n",
		tester.total, tester.success, tester.fail, tester.skip, tester.bad)
}

type testRunner struct {
	baseDir string

	// stats
	total   int
	success int
	skip    int
	fail    int
	bad     int
}

type testCase struct {
	Disable bool
	File    string
	Modules []string
	TestDir string
	Tests   []*testCase
}

type testOutput struct {
	pos  token.Position
	text string
}

type outputKind int

const (
	unknownOutput outputKind = iota
	exeOutput
	compilerOutput
)

type testResult struct {
	status status
	reason []string
}

func (r *testResult) addReason(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	r.reason = append(r.reason, msg)
}

type status int

const (
	statusSuccess status = iota
	statusFail
	statusSkip
	statusBad
)

func (t status) String() string {
	switch t {
	case statusSuccess:
		return "OK"
	case statusFail:
		return "FAIL"
	case statusSkip:
		return "SKIP"
	case statusBad:
		return "BAD"
	default:
		return "-"
	}
}

func abort(err error) {
	fmt.Println("error:", err)
	os.Exit(1)
}

func readTestManifest(manifest string) []*testCase {
	bytes, err := ioutil.ReadFile(manifest)
	if err != nil {
		abort(err)
	}

	var tests []*testCase

	err = json.Unmarshal(bytes, &tests)
	if err != nil {
		abort(err)
	}

	return tests
}

func createTests(testFiles []string) []*testCase {
	var tests []*testCase

	for _, testFile := range testFiles {
		test := &testCase{}
		test.File = testFile
		test.TestDir = ""

		tests = append(tests, test)
	}

	return tests
}

func (t *testRunner) runTests(baseDir string, tests []*testCase) {
	for _, test := range tests {
		if test.Disable {
			t.updateStats(statusSkip)
			continue
		}

		validTest := false

		if len(test.File) > 0 {
			validTest = true

			ext := filepath.Ext(test.File)
			baseName := filepath.Base(test.File)
			baseName = baseName[:len(baseName)-len(ext)]
			testName := filepath.Join(baseDir, baseName)

			result := t.runTest(testName, baseDir, test)
			t.updateStats(result.status)

			fmt.Printf("test %s%s[%s]\n", testName, strings.Repeat(".", 50-len(testName)), result.status)

			for _, txt := range result.reason {
				fmt.Printf("  >> %s\n", txt)
			}
		}

		if len(test.Tests) > 0 {
			validTest = true
			testDir := filepath.Join(baseDir, test.TestDir)
			t.runTests(testDir, test.Tests)
		}

		if !validTest {
			t.updateStats(statusBad)
		}
	}
}

func (t *testRunner) updateStats(res status) {
	t.total++
	switch res {
	case statusSuccess:
		t.success++
	case statusFail:
		t.fail++
	case statusSkip:
		t.skip++
	case statusBad:
		t.bad++
	}
}

func (t *testRunner) runTest(testName string, testDir string, test *testCase) *testResult {
	var filenames []string
	filenames = append(filenames, filepath.Join(t.baseDir, testDir, test.File))

	for _, mod := range test.Modules {
		filename := filepath.Join(t.baseDir, testDir, mod)
		filenames = append(filenames, filename)
	}

	var expectedCompilerOutput []*testOutput
	var expectedExeOutput []*testOutput
	errors := &common.ErrorList{}

	result := &testResult{status: statusSuccess}
	set, err := module.Load(filenames)

	if set != nil {
		if mod := set.FindModule("main"); mod != nil {
			file := mod.FindFileWithFQN("main")
			expectedCompilerOutput, expectedExeOutput = parseTestDescription(file.Comments)
		} else {
			result.status = statusFail
			result.addReason("internal error: no main module")
			return result
		}
	}

	config := common.NewBuildConfig()
	config.Exe = filepath.Join(os.TempDir(), strings.Replace(testName, "/", "_", -1))

	if !addError(err, errors) {
		err = semantics.Check(set)
		if !addError(err, errors) {
			err = backend.Build(set, config)
			addError(err, errors)
		}
	}

	var compilerOutput []*testOutput

	errors.Sort()
	addCompilerOutput(errors.Warnings, &compilerOutput)
	addCompilerOutput(errors.Errors, &compilerOutput)
	compareOutput(expectedCompilerOutput, compilerOutput, result)

	var exeOutput []*testOutput

	if !errors.IsError() {
		cmd := exec.Command(config.Exe)
		bytes, err := cmd.CombinedOutput()
		if err != nil {
			result.addReason("internal error: %s", err)
		} else {
			addExeOutput(bytes, &exeOutput)
		}
	}

	compareOutput(expectedExeOutput, exeOutput, result)

	if len(result.reason) > 0 {
		result.status = statusFail
	}

	os.Remove(config.Exe)

	return result
}

func addError(newError error, errors *common.ErrorList) bool {
	if newError == nil {
		return false
	}
	errors.AddGeneric1(newError)
	return errors.IsError()
}

func addCompilerOutput(errors []*common.Error, output *[]*testOutput) {
	for _, err := range errors {
		pos := err.Pos

		msg := fmt.Sprintf("%s(%d:%d): %s", err.ID, pos.Line, pos.Column, err.Msg)
		*output = append(*output, &testOutput{pos: pos, text: msg})

		for _, line := range err.Context {
			*output = append(*output, &testOutput{pos: pos, text: line})
		}
	}
}

func addExeOutput(bytes []byte, output *[]*testOutput) {
	tmp := strings.Split(string(bytes), "\n")
	for i, line := range tmp {
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			pos := token.Position{Line: i + 1, Column: 1}
			*output = append(*output, &testOutput{pos: pos, text: line})
		}
	}
}

func compareOutput(expectedOutput []*testOutput, actualOutput []*testOutput, result *testResult) {
	expectedIdx := 0
	actualIdx := 0
	regexPrefix := "re:"

	for ; expectedIdx < len(expectedOutput); expectedIdx++ {
		expected := expectedOutput[expectedIdx]

		if actualIdx >= len(actualOutput) {
			break
		}

		actual := actualOutput[actualIdx]
		actualIdx++
		match := true

		if strings.HasPrefix(expected.text, regexPrefix) {
			pattern := strings.TrimSpace(expected.text[len(regexPrefix):])
			regex, err := regexp.Compile(pattern)
			if err != nil {
				result.addReason("bad regex: %s: %s", expected.pos, err)
			} else {
				match = regex.MatchString(actual.text)
			}
		} else {
			match = expected.text == actual.text
		}

		if !match {
			result.addReason("expected(%s): %s", expected.pos, expected.text)
			result.addReason("  got(%s): %s", actual.pos, actual.text)
		}
	}

	if actualIdx < len(actualOutput) {
		for i := actualIdx; i < len(actualOutput); i++ {
			result.addReason("got(%s): %s", actualOutput[i].pos, actualOutput[i].text)
		}
	}

	if expectedIdx < len(expectedOutput) {
		for i := expectedIdx; i < len(expectedOutput); i++ {
			result.addReason("expected(%s): %s", expectedOutput[i].pos, expectedOutput[i].text)
		}
	}
}

func parseTestDescription(comments []*ir.Comment) (compiler []*testOutput, exe []*testOutput) {
	compilerPrefix := "expect-dgc:"
	exePrefix := "expect-exe:"

	for _, comment := range comments {
		// Ignore multi-line comments
		if comment.Tok.Is(token.Comment) {
			lit := comment.Literal[2:]
			lit = strings.TrimSpace(lit)

			if strings.HasPrefix(lit, compilerPrefix) {
				lit = strings.TrimSpace(lit[len(compilerPrefix):])
				compiler = append(compiler, &testOutput{pos: comment.Tok.Pos, text: lit})
			} else if strings.HasPrefix(lit, exePrefix) {
				lit = strings.TrimSpace(lit[len(exePrefix):])
				exe = append(exe, &testOutput{pos: comment.Tok.Pos, text: lit})
			}
		}
	}

	return compiler, exe
}