package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"testing"
)

var (
	ErrTest = errors.New("test error")
)

func TestRun(t *testing.T) {
	type buildInfo struct {
		version, commitHash, buildDate string
	}

	type basetest struct {
		// params
		name           string
		skip           bool
		args           []string
		stdout, stderr io.Writer
		buildInfo      buildInfo
		openFile       func(name string) (io.WriteCloser, error)
		// expectations
		checkStdout func(t *testing.T, stdout *recordFile)
		checkStderr func(t *testing.T, stderr *recordFile)
		checkFile   func(t *testing.T, file *recordFile)
		checkRename func(t *testing.T, wasRenamed bool)
		wantCode    int
		wantErr     error
	}

	runTest := func(t *testing.T, tt basetest) {
		if tt.skip {
			t.Skip()
		}
		ctx := context.Background()

		var file io.WriteCloser
		if tt.checkFile != nil {
			openFileFn := tt.openFile
			tt.openFile = func(name string) (io.WriteCloser, error) {
				f, err := openFileFn(name)
				if err != nil {
					return nil, err
				}
				file = f
				return f, nil
			}
		}

		var wasRenamed bool
		if tt.checkRename != nil {
			if tt.openFile == nil {
				tt.openFile = mockfile(nil, nil, func() {
					wasRenamed = true
				})
			} else {
				openFileFn := tt.openFile
				tt.openFile = func(name string) (io.WriteCloser, error) {
					f, err := openFileFn(name)
					if err != nil {
						// error, no need to check rename
						return nil, err
					}
					v, ok := f.(*mockFile)
					if !ok {
						t.Errorf("can't check rename: got %T, want *mockFile", f)
					}
					v.withRenameFn = func() {
						wasRenamed = true
					}
					return f, nil
				}
			}
		}

		code, err := run(
			ctx,
			tt.args,
			tt.stdout,
			tt.stderr,
			tt.buildInfo.version,
			tt.buildInfo.commitHash,
			tt.buildInfo.buildDate,
			tt.openFile,
		)
		if tt.checkStdout != nil {
			if stdout, ok := tt.stdout.(*recordFile); !ok {
				t.Errorf("can't check stdout: got %T, want *recordFile", tt.stdout)
			} else {
				tt.checkStdout(t, stdout)
			}
		}
		if tt.checkStderr != nil {
			if stderr, ok := tt.stderr.(*recordFile); !ok {
				t.Errorf("can't check stderr: got %T, want *recordFile", tt.stderr)
			} else {
				tt.checkStderr(t, stderr)
			}
		}
		if tt.checkFile != nil {
			if f, ok := file.(*recordFile); !ok {
				t.Errorf("can't check file: got %T, want *recordFile", file)
			} else {
				tt.checkFile(t, f)
			}
		}
		if tt.checkRename != nil {
			tt.checkRename(t, wasRenamed)
		}
		if code != tt.wantCode {
			t.Errorf("run() code = %v, want %v", code, tt.wantCode)
		}
		if err != tt.wantErr {
			if err == nil || tt.wantErr == nil || !strings.Contains(err.Error(), tt.wantErr.Error()) {
				t.Errorf("run() err = %v, want %v", err, tt.wantErr)
			}
		}
	}

	t.Run("invalid usage", func(t *testing.T) {
		for _, tt := range []basetest{
			{
				name:        "unknown command",
				args:        []string{"cmd"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				checkStderr: requireContent("regexp", "^invalid flags: output file is required"),
				wantCode:    2,
			},
			{
				name:   "unknown flag & version cmd",
				args:   []string{"version", "--undefined-flag", "value"},
				stdout: &recordFile{},
				stderr: &recordFile{},
				buildInfo: buildInfo{
					version:    "v1.2.3",
					commitHash: "main",
					buildDate:  "2025-01-01",
				},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^flag provided but not defined"),
			},
			{
				name:        "unknown flag & root cmd",
				args:        []string{"-output", "output.go", "-package", "gen", "-field", "UserID", "--undefined-flag", "value"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^flag provided but not defined"),
			},
			{
				name:        "root cmd & missed output",
				args:        []string{"-package", "gen", "-field", "UserID"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^invalid flags: output file is required"),
			},
			{
				name:        "root cmd & missed package",
				args:        []string{"-output", "output.go", "-field", "UserID"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^invalid flags: package name is required"),
			},
			{
				name:        "root cmd & missed field",
				args:        []string{"-output", "output.go", "-package", "gen"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^invalid flags: at least one field is required"),
			},
			{
				name:        "root cmd & field full duplicate",
				args:        []string{"-output", "output.go", "-package", "gen", "-field", "UserID", "-field", "UserID"},
				stdout:      &recordFile{},
				stderr:      &recordFile{},
				wantCode:    2,
				checkStderr: requireContent("regexp", "^invalid fields: field .* is duplicated"),
			}, {
				name:     "root cmd & field partial duplicate",
				args:     []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string", "-field", "UserID:int"},
				stdout:   &recordFile{},
				stderr:   &recordFile{},
				wantCode: 2,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				runTest(t, tt)
			})
		}
	})

	t.Run("sub-cmds", func(t *testing.T) {
		for _, tt := range []basetest{
			{
				name:   "version",
				args:   []string{"version"},
				stdout: &recordFile{},
				stderr: &recordFile{},
				buildInfo: buildInfo{
					version:    "v1.2.3",
					commitHash: "main",
					buildDate:  "2025-01-01",
				},
				checkStdout: requireContent("regexp", `.*
Version:     v1.2.3
Build Date:  2025-01-01
Commit Hash: main
$`),
				wantCode: 0,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				runTest(t, tt)
			})
		}
	})

	t.Run("root-cmd", func(t *testing.T) {
		t.Run("filesystem", func(t *testing.T) {
			for _, tt := range []basetest{
				{
					name:   "file open error",
					args:   []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string"},
					stdout: ioutil.Discard,
					stderr: ioutil.Discard,
					openFile: func(name string) (io.WriteCloser, error) {
						return nil, ErrTest
					},
					wantCode: 1,
					wantErr:  ErrTest,
				},
				{
					name:   "file close error",
					args:   []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string"},
					stdout: &recordFile{},
					stderr: &recordFile{},
					openFile: mockfile(nil, func() error {
						return ErrTest
					}, nil),
					wantCode: 1,
					wantErr:  ErrTest,
				},
				{
					name:   "file write error",
					args:   []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string"},
					stdout: &recordFile{},
					stderr: &recordFile{},
					openFile: mockfile(func(p []byte) (n int, err error) {
						return 0, ErrTest
					}, nil, nil),
					wantCode: 1,
					wantErr:  ErrTest,
				},
				{
					name:        "file must be renamed on gen success",
					args:        []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string"},
					stdout:      ioutil.Discard,
					stderr:      ioutil.Discard,
					openFile:    mockfile(nil, nil, nil),
					checkRename: withRename,
					wantCode:    0,
				},
				{
					name:   "file must not be renamed on gen error",
					args:   []string{"-output", "output.go", "-package", "gen", "-field", "UserID:string"},
					stdout: ioutil.Discard,
					stderr: ioutil.Discard,
					openFile: mockfile(func(p []byte) (n int, err error) {
						return 0, ErrTest
					}, nil, nil),
					checkRename: withoutRename,
					wantCode:    1,
					wantErr:     ErrTest,
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					runTest(t, tt)
				})
			}
		})

		t.Run("invalid fields", func(t *testing.T) {
			for _, tt := range []basetest{
				{
					name: "not built-in types #1",
					args: []string{
						"-output", "output.go",
						"-package", "gen",
						"-field", "Field1:invalid",
						"-field", "Field2:strings",
						"-field", "Field3:int[]",
						"-field", "Field4:[]int[]",
						"-field", "Field5:3+3*3",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 2,
				},
				{
					name: "not built-in types #2",
					args: []string{
						"-output", "output.go",
						"-package", "gen",
						"-field", "Field2:strings",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 2,
				},
				{
					name: "not built-in types #3",
					args: []string{
						"-output", "output.go",
						"-package", "gen",
						"-field", "Field3:int[]",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 2,
				},
				{
					name: "not built-in types #4",
					args: []string{
						"-output", "output.go",
						"-package", "gen",
						"-field", "Field4:[]int[]",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 2,
				},
				{
					name: "not built-in types #5",
					args: []string{
						"-output", "output.go",
						"-package", "gen",
						"-field", "Field5:3+3*3",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 2,
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					runTest(t, tt)
				})
			}
		})

		t.Run("basic usage", func(t *testing.T) {
			for _, tt := range []basetest{
				{
					name:     "default field type must be interface{}",
					args:     []string{"-output", "output.go", "-package", "gen", "-field", "UserID"},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: record,
					checkFile: requireContent("eq", `// Code generated by valctx . DO NOT EDIT.

package gen

import (
    "context"
)

type userIDKey struct{}

// Get UserID retrieves the UserID from the context.
func GetUserID(ctx context.Context) interface{} {
    v := ctx.Value(userIDKey{})
    return v
}

// SetUserID sets the UserID in the context.
func SetUserID(ctx context.Context, v interface{}) context.Context {
    return context.WithValue(ctx, userIDKey{}, v)
}
`),
					wantCode: 0,
				},
				{
					name: "fields with built-in types",
					args: []string{
						"-output", "output.go", "-package", "gen",
						"-field", "Field1:int8",
						"-field", "Field2:[]string",
						"-field", "Field3:map[string]rune",
						"-field", "Field4:[3]interface{}",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 0,
				},
				{
					name: "fields with exported types",
					args: []string{
						"-output", "output.go", "-package", "gen",
						"-field", "Field1:github.com/user/pkg.User",
						"-field", "Field2:context.Context",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: devnull,
					wantCode: 0,
				},
				{
					name: "field name capitalized (unexported to exported)",
					args: []string{
						"-output", "output.go", "-package", "gen",
						"-field", "field1:int",
					},
					stdout:   ioutil.Discard,
					stderr:   ioutil.Discard,
					openFile: record,
					checkFile: requireContent("eq", `// Code generated by valctx . DO NOT EDIT.

package gen

import (
    "context"
)

type field1Key struct{}

// Get Field1 retrieves the Field1 from the context.
func GetField1(ctx context.Context) (int, bool) {
    v, ok := ctx.Value(field1Key{}).(int)
    return v, ok
}

// SetField1 sets the Field1 in the context.
func SetField1(ctx context.Context, v int) context.Context {
    return context.WithValue(ctx, field1Key{}, v)
}
`),
					wantCode: 0,
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					runTest(t, tt)
				})
			}
		})
	})
}

type recordFile struct {
	Name string
	Data bytes.Buffer
}

func (r *recordFile) Write(p []byte) (n int, err error) {
	return r.Data.Write(p)
}

func (r *recordFile) Close() error {
	return nil
}

type discardFile struct{}

func (discardFile) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (discardFile) Close() error {
	return nil
}

type mockFile struct {
	writeFn      func(p []byte) (n int, err error)
	closeFn      func() error
	withRenameFn func()
}

func (m *mockFile) Write(p []byte) (n int, err error) {
	if m.writeFn == nil {
		return len(p), nil
	}
	return m.writeFn(p)
}

func (m *mockFile) Close() error {
	if m.closeFn == nil {
		return nil
	}
	return m.closeFn()
}

func (m *mockFile) WithRename() {
	if m.withRenameFn == nil {
		return
	}
	m.withRenameFn()
}

func devnull(_ string) (io.WriteCloser, error) {
	return discardFile{}, nil
}

func record(_ string) (io.WriteCloser, error) {
	return &recordFile{}, nil
}

func mockfile(
	writeFn func(p []byte) (n int, err error),
	closeFn func() error,
	withRenameFn func(),
) func(_ string) (io.WriteCloser, error) {
	return func(_ string) (io.WriteCloser, error) {
		return &mockFile{
			writeFn:      writeFn,
			closeFn:      closeFn,
			withRenameFn: withRenameFn,
		}, nil
	}
}

func requireContent(op string, pattern string) func(*testing.T, *recordFile) {
	return func(t *testing.T, f *recordFile) {
		if strings.TrimSpace(pattern) == "" {
			if f.Data.Len() != 0 {
				t.Errorf("content: got %q, want empty", f.Data.String())
			}
			return
		}
		switch op {
		case "eq":
			if f.Data.String() != pattern {
				t.Errorf("content: got %q, want %q", f.Data.String(), pattern)
			}
		default: // regexp
			re, err := regexp.Compile(pattern)
			if err != nil {
				t.Errorf("content: invalid pattern: %v", err)
				return
			}
			if !re.Match(f.Data.Bytes()) {
				t.Errorf("content: got %q, want %q", f.Data.String(), pattern)
			}
		}
	}
}

func withRename(t *testing.T, wasRenamed bool) {
	if !wasRenamed {
		t.Error("file was not renamed, but expected")
	}
}

func withoutRename(t *testing.T, wasRenamed bool) {
	if wasRenamed {
		t.Error("file was renamed, but not expected")
	}
}
