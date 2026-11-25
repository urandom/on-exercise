package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/urandom/on-exercise/internal/app"
	"github.com/urandom/on-exercise/internal/hr"
	"github.com/urandom/on-exercise/internal/policy"
	"github.com/urandom/on-exercise/internal/state"
	"github.com/urandom/on-exercise/internal/store"
)

var (
	fApply  = flag.Bool("apply", false, "set to true to apply the plan")
	fFailOn = flag.String("fail-on", "", "fail the first write for `employee_id` without performing it "+
		"(for demonstrating failure recovery)")

	// Input files, opened and validated during flag parsing.
	hrFile, appFile, policyFile *os.File
)

func init() {
	flag.Func("hr", "path to the HR snapshot JSON file (required)", fileFlag(&hrFile))
	flag.Func("app", "path to the application state JSON file (required)", fileFlag(&appFile))
	flag.Func("policy", "path to the department to group policy JSON file (required)", fileFlag(&policyFile))

	flag.Usage = usage
}

func main() {
	flag.Parse()

	// Usage errors exit with 2, matching the flag package's own behaviour.
	if err := validateArgs(); err != nil {
		fmt.Fprintln(flag.CommandLine.Output(), err)
		flag.Usage()
		os.Exit(2)
	}

	if err := run(); err != nil {
		slog.Error("Error running app", slog.String("error", err.Error()))

		os.Exit(1)
	}
}

func run() error {
	for _, f := range []*os.File{hrFile, appFile, policyFile} {
		if f != nil {
			defer f.Close()
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	reconciler := app.New(
		failingStore{JSONStore: store.NewJSONStore(appFile), target: state.EmployeeID(*fFailOn)},
		hr.NewLoader(hrFile),
		policy.NewLoader(policyFile),
		slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	)

	res, runErr := reconciler.Run(ctx, *fApply)

	// The plan is only set once the inputs have been validated. A failed
	// apply always has at least one action (the write that failed), so an
	// empty plan alongside an error means the run stopped before planning
	// and there is nothing meaningful to print.
	if runErr == nil || len(res.Plan.Actions) > 0 {
		if err := printResult(os.Stdout, res, *fApply, runErr == nil); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("printing plan: %w", err))
		}
	}

	if runErr != nil {
		return fmt.Errorf("run %s: %w", res.ID, runErr)
	}

	return nil
}

// printResult writes the run's plan to w, prefixed with the run ID and mode
// and, for an apply with actions, followed by whether it completed.
func printResult(w io.Writer, res app.Result, apply, succeeded bool) error {
	mode := "plan only, nothing written; rerun with -apply to apply it"
	if apply {
		mode = "apply"
	}
	if _, err := fmt.Fprintf(w, "Run %s (%s)\n", res.ID, mode); err != nil {
		return err
	}

	if _, err := res.Plan.WriteTo(w); err != nil {
		return err
	}

	if !apply || len(res.Plan.Actions) == 0 {
		return nil
	}

	status := "All actions applied."
	if !succeeded {
		status = "Apply failed; actions after the failed one were not attempted. " +
			"Fix the cause and rerun to complete the remaining actions."
	}
	_, err := fmt.Fprintln(w, status)

	return err
}

// validateArgs rejects stray positional arguments and reports every required
// file flag that was not provided. flag.Func handlers only run for flags that
// are present, so absence has to be checked after parsing.
func validateArgs() error {
	required := []struct {
		name string
		file *os.File
	}{{"hr", hrFile}, {"app", appFile}, {"policy", policyFile}}

	var errs []error
	// Boolean flags only accept "-apply=false"; "-apply false" sets apply to
	// true and leaves "false" as a positional argument. Reject any positional
	// arguments so that mistake cannot silently write changes.
	if flag.NArg() > 0 {
		errs = append(errs, fmt.Errorf("unexpected arguments: %q (use -flag=value for boolean flags)", flag.Args()))
	}
	for _, r := range required {
		if r.file == nil {
			errs = append(errs, fmt.Errorf("-%s is required", r.name))
		}
	}

	return errors.Join(errs...)
}

// fileFlag returns a flag.Func handler that opens the given path for reading
// and stores the file in dst. It rejects empty paths, repeated flags, and
// anything that is not a regular file.
func fileFlag(dst **os.File) func(string) error {
	return func(path string) error {
		if path == "" {
			return errors.New("path must not be empty")
		}
		if *dst != nil {
			return errors.New("flag specified more than once")
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}

		// Stat the opened file rather than the path, so the check applies to
		// the file we will actually read.
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			f.Close()
			return fmt.Errorf("%s is not a regular file", path)
		}

		*dst = f
		return nil
	}
}

func usage() {
	name := filepath.Base(os.Args[0])
	out := flag.CommandLine.Output()

	fmt.Fprintf(out, `Reconcile application access with the HR snapshot and department policy.

Prints a read-only plan by default. Changes are only written to the
application state file when -apply is given.

Usage:
  %[1]s -hr FILE -app FILE -policy FILE [-apply] [-fail-on EMPLOYEE_ID]

Examples:
  %[1]s -hr hr.json -app app.json -policy policy.json          # show plan
  %[1]s -hr hr.json -app app.json -policy policy.json -apply   # apply plan
  %[1]s -hr hr.json -app app.json -policy policy.json -apply -fail-on=E004
                                     # apply, failing the first write for E004

Flags:
`, name)
	flag.PrintDefaults()
}
