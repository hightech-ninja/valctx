package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hightech-ninja/valctx/internal/app"
	"github.com/hightech-ninja/valctx/internal/gen"
)

// Version, CommitHash, BuildDate are set with ldflags.
var (
	Version    = "latest"
	CommitHash = "main"
	BuildDate  = "unknown"
)

func main() {
	ctx, cancel := app.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()
	code, err := run(
		ctx,
		os.Args[1:], os.Stdout, os.Stderr,
		Version, CommitHash, BuildDate,
		app.NewSafeFile,
	)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	if code != 0 {
		os.Exit(code)
	}
}

func run(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	version, commitHash, buildDate string,
	openFile func(name string) (io.WriteCloser, error),
) (int, error) {
	generate := func(
		outFile string,
		pkg gen.Package,
		fs []gen.Field,
	) (err error) {
		file, err := openFile(outFile)
		if err != nil {
			return fmt.Errorf("open file: %v", err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil && err == nil {
				err = closeErr
			} else if closeErr != nil && err != nil {
				err = fmt.Errorf("%v; %v", err, closeErr)
			}
		}()
		err = gen.Generate(ctx, file, pkg, fs)
		if err != nil {
			return fmt.Errorf("generate: %v", err)
		}
		type renamer interface {
			WithRename()
		}
		if r, ok := file.(renamer); ok {
			r.WithRename()
		}
		return nil
	}

	versionCmd := flag.NewFlagSet("version", flag.ContinueOnError)
	versionCmd.SetOutput(stderr)
	versionCmd.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage: valctx version")
		versionCmd.PrintDefaults()
	}

	rootCmd := flag.NewFlagSet("", flag.ContinueOnError)
	rootCmd.SetOutput(stderr)
	rootCmd.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "\nUsage: valctx [flags]")
		_, _ = fmt.Fprintln(stderr, "Valctx is a tool to generate convenient setters and getters for context values.")
		rootCmd.PrintDefaults()
	}
	var (
		output string
		pkg    string
		fields app.FieldFlags
	)
	rootCmd.StringVar(&output, "output", "", "Output file.")
	rootCmd.StringVar(&pkg, "package", "", "Package name for the generated file.")
	rootCmd.Var(&fields, "field", "Context field in go-code format, but name and type separated with colon.\n\t"+
		"All fields must have unique names. There are some limitations on allowed types.\n\t"+
		"Examples:\n\t\t* UserID:int\n\t\t* Data:[]string\n\t\t* User:github.com/user/pkg.User")
	validateRootCmdFlags := func() error {
		if output == "" {
			return fmt.Errorf("output file is required")
		}
		if pkg == "" {
			return fmt.Errorf("package name is required")
		}
		if len(fields) == 0 {
			return fmt.Errorf("at least one field is required")
		}
		return nil
	}

	var subCmd string
	if len(args) > 0 {
		subCmd = args[0]
	}

	switch {
	default: // root
		if err := rootCmd.Parse(args); err != nil {
			return 2, nil
		}
		if err := validateRootCmdFlags(); err != nil {
			_, _ = fmt.Fprintf(stderr, "invalid flags: %v\n", err)
			rootCmd.Usage()
			return 2, nil
		}
		genPkg, genFields, err := app.ParseFields(pkg, version, fields)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "invalid fields: %v\n", err)
			rootCmd.Usage()
			return 2, nil
		}
		if err = generate(output, genPkg, genFields); err != nil {
			return 1, err
		}
	case subCmd == "version":
		if err := versionCmd.Parse(args[1:]); err != nil {
			return 2, nil
		}
		if _, err := fmt.Fprintf(stdout, `Valctx is a tool to generate convenient setters and getters for context values.
Version:     %s
Build Date:  %s
Commit Hash: %s
`, version, buildDate, commitHash); err != nil {
			return 1, err
		}
	}
	return 0, nil
}
