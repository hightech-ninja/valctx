package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hightech-ninja/valctx/internal/gen"
)

var (
	ErrInvalidFormat         = errors.New("invalid format")
	ErrUnsupportedFlagFormat = errors.New("unsupported flag format")
)

type FieldKind int

const (
	FieldKindDefault FieldKind = iota
	FieldKindBuiltInOnly
	FieldKindCustomType
)

type FieldFlag struct {
	Kind FieldKind
	Name string
	Type string
}

func (f *FieldFlag) String() string {
	switch {
	case f == nil:
		return ""
	case f.Kind == FieldKindDefault:
		return f.Name
	default:
		return fmt.Sprintf("%s:%s", f.Name, f.Type)
	}
}

func NewField(value string) (FieldFlag, error) {
	var f FieldFlag
	parts := strings.SplitN(value, ":", 2)
	switch len(parts) {
	default:
		return FieldFlag{}, ErrInvalidFormat
	case 1: // Name
		parts[0] = strings.TrimSpace(parts[0])
		f = FieldFlag{
			Kind: FieldKindDefault,
			Name: modifyFirstLetter(parts[0], strings.ToUpper),
		}
	case 2: // Name[:Type]
		parts[0] = strings.TrimSpace(parts[0])
		parts[1] = strings.TrimSpace(parts[1])
		dot := strings.LastIndex(parts[1], ".")
		slash := strings.LastIndex(parts[1], "/")
		if dot > slash {
			f = FieldFlag{
				Kind: FieldKindCustomType,
				Name: modifyFirstLetter(parts[0], strings.ToUpper),
				Type: strings.TrimSpace(parts[1]),
			}
		} else if dot == -1 {
			f = FieldFlag{
				Kind: FieldKindBuiltInOnly,
				Name: modifyFirstLetter(parts[0], strings.ToUpper),
				Type: strings.TrimSpace(parts[1]),
			}
		} else {
			return FieldFlag{}, ErrInvalidFormat
		}
	}

	return f, f.Validate()
}

func (f *FieldFlag) Validate() error {
	if f == nil {
		return ErrInvalidFormat
	}
	if f.Name == "" {
		return errors.New("name is required")
	}
	if f.Kind == FieldKindDefault {
		return nil
	}
	if f.Type == "" {
		return errors.New("type is required")
	}
	if f.Kind == FieldKindBuiltInOnly || f.Kind == FieldKindCustomType {
		return nil
	}
	return ErrUnsupportedFlagFormat
}

type FieldFlags []FieldFlag

func (a *FieldFlags) String() string {
	if a == nil {
		return ""
	}
	fields := make([]string, 0, len(*a))
	for _, f := range *a {
		fields = append(fields, f.String())
	}
	return strings.Join(fields, ",")
}

func (a *FieldFlags) Set(value string) error {
	f, err := NewField(value)
	if err != nil {
		return err
	}
	*a = append(*a, f)
	return nil
}

func ParseFields(pkg, version string, fs FieldFlags) (gen.Package, []gen.Field, error) {
	genFields := make([]gen.Field, 0, len(fs))
	seenFields := map[string]struct{}{}
	seenPkgs := map[string]struct{}{
		"context": {},
	}
	for _, f := range fs {
		field := gen.Field{
			FieldName: f.Name,
			KeyName:   modifyFirstLetter(f.Name, strings.ToLower) + "Key",
		}
		switch f.Kind {
		case FieldKindDefault:
			field.FieldType = "interface{}"
		case FieldKindBuiltInOnly:
			field.FieldType = f.Type
		case FieldKindCustomType:
			dot := strings.LastIndex(f.Type, ".")
			pkgName := f.Type[:dot]
			seenPkgs[pkgName] = struct{}{}
			slash := strings.LastIndex(pkgName, "/")
			field.SetPackage(pkgName)
			field.FieldType = f.Type[slash+1:]
		default:
			return gen.Package{}, nil, ErrUnsupportedFlagFormat
		}

		err := field.Validate()
		if err != nil {
			return gen.Package{}, nil, fmt.Errorf("invalid field %q: %v", f.Name, err)
		}

		_, seen := seenFields[field.FieldName]
		if seen {
			return gen.Package{}, nil, fmt.Errorf("field %q is duplicated", field.FieldName)
		}

		seenFields[field.FieldName] = struct{}{}
		genFields = append(genFields, field)
	}

	toImport := make([]string, 0, len(seenPkgs))
	for imp := range seenPkgs {
		toImport = append(toImport, imp)
	}
	sort.Sort(sort.StringSlice(toImport))

	genPkg := gen.Package{
		PackageName:    pkg,
		ImportPackages: toImport,
		Version:        version,
	}
	if err := genPkg.Validate(); err != nil {
		return gen.Package{}, nil, err
	}

	return genPkg, genFields, nil
}

func modifyFirstLetter(s string, modify func(string) string) string {
	r, size := utf8.DecodeRuneInString(s)
	return modify(string(r)) + s[size:]
}

// SafeFile is a temporary file that is renamed to the output file on Close, but only if
// WithRename is called before Close. Otherwise and in case of errors, the temporary file is removed.
// If output file already exists, it is overwritten.
type SafeFile struct {
	*os.File
	output    string
	rmOnClose bool
}

func NewSafeFile(output string) (io.WriteCloser, error) {
	output = filepath.Clean(output)
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return nil, fmt.Errorf("output is a directory")
	}
	outputDir := filepath.Dir(output)
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("create output filepath: %v", err)
	}

	temp, err := ioutil.TempFile(outputDir, "valctx-")
	if err != nil {
		return nil, fmt.Errorf("create temporary file: %v", err)
	}

	return &SafeFile{
		File:      temp,
		output:    output,
		rmOnClose: true,
	}, nil
}

func (f *SafeFile) WithRename() {
	f.rmOnClose = false
}

func (f *SafeFile) Close() error {
	err := f.File.Close()
	if err != nil {
		_ = os.Remove(f.File.Name())
		return fmt.Errorf("close temporary file: %v", err)
	}
	if f.rmOnClose {
		err = os.Remove(f.File.Name())
		if err != nil {
			return fmt.Errorf("remove temporary file: %v", err)
		}
		return nil
	}
	err = os.Rename(f.File.Name(), f.output)
	if err != nil {
		_ = os.Remove(f.File.Name())
		return fmt.Errorf("rename to output file: %v", err)
	}
	return nil
}

func NotifyContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, signals...)
		defer signal.Stop(sigCh)
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
