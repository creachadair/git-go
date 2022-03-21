// Copyright (C) 2016 Michael J. Fromberger. All Rights Reserved.

// Program git-go is a Git plugin that adds some helpful behaviour for
// repositories containing Go code.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/creachadair/ctrl"
)

var (
	modDirs = flag.String("mod", "auto", "Module paths relative to repository root")

	out = os.Stdout
)

func init() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `Usage: git go [subcommand] [options]

Helpful additions for writing and maintaining Go code.

Subcommands:
  presubmit     : run "gofmt", "go test", and "go vet" over all packages
  test, tests   : run "go test" over all packages
  test-once     : as "test", but limited to one CPU setting
  vet           : run "go vet" over all packages
  fmt, format   : run "gofmt -s" over all packages
  static        : run "staticheck" over all packages (if installed)
  check         : run all the above checks

  install-tools : install external commands (staticcheck)

  install-hook [subcommand]
                : install pre-push hook in the current repo.
                  subcommand defaults to "presubmit"

  install-presubmit-workflow
                : install presubmit workflow config in the current repo.

Set GITGO_<tag>=warn to convert failures into warnings, where tag is one of
  TEST, VET, FMT, STATIC

When using "presubmit" or "check", additional arguments are added to or removed
from the base set, e.g., "presubmit static" means fmt, test, vet, and static,
while "check -vet" means all the tests except vet.

By default, all submodules are checked. To check only specific submodules, set
the -mod flag to a comma-separated list of subdirectories of the main module.
To check only the main module, set -mod="."

Options:
`)
		flag.PrintDefaults()
	}
}
func main() {
	flag.Parse()
	ctrl.Run(gitgo)
}

func gitgo() error {
	if flag.NArg() == 0 {
		return errors.New("no subcommand specified")
	}

	// Special cases: Install hooks, fetch tools, etc.
	if flag.Arg(0) == "install-hook" {
		root, err := rootDir()
		if err != nil {
			return err
		}
		subcommand := "presubmit"
		if flag.NArg() > 1 {
			subcommand = flag.Arg(1)
		}
		hookdir := filepath.Join(root, ".git", "hooks")
		prepush := filepath.Join(hookdir, "pre-push")
		if _, err := os.Stat(prepush); os.IsNotExist(err) {
			return writeHook(prepush, subcommand, *modDirs)
		} else if err == nil {
			return fmt.Errorf("pre-push hook already exists")
		} else {
			return err
		}
	} else if flag.Arg(0) == "install-tools" {
		return installTools()
	} else if flag.Arg(0) == "install-presubmit-workflow" {
		return installPresubmitWorkflow()
	} else if flag.Arg(0) == "help" {
		flag.Usage()
		return nil
	}

	// Reaching here, we have checking to do.
	root, err := moduleRoot()
	if err != nil {
		return err
	} else if err := os.Chdir(root); err != nil {
		return err
	}
	mods, err := findSubmodules(root, *modDirs)
	if err != nil {
		return err
	}

	args := flag.Args()
	if len(args) >= 1 {
		fix := args[1:]
		switch args[0] {
		case "check":
			args = []string{"fmt", "test", "vet", "static"}
		case "presubmit":
			args = []string{"fmt", "test", "vet"}
		}
		for _, arg := range fix {
			update(&args, arg)
		}
	}
	var nerr int
	for _, mod := range mods {
		if _, err := os.Stat(filepath.Join(mod, "go.mod")); err != nil {
			return fmt.Errorf("no module in %q: %w", mod, err)
		}
		fmt.Fprintf(out, "🛠  \033[1;97mChecking module %q\033[0m\n", mod)
		for _, arg := range args {
			err := func() error {
				switch arg {
				case "test", "tests":
					return check("test", invoke(runTests(mod)))

				case "test-once":
					return check("test", invoke(runTestsOnce(mod)))

				case "vet":
					return check("vet", invoke(runVet(mod)))

				case "static":
					if isGo118() {
						fmt.Fprintf(out, "▷ \033[1;36m%s\033[0m\n", "staticcheck")
						fmt.Fprintf(out, "\033[50C\033[1;33mSKIPPED\033[0m (Go 1.18 is not supported yet)\n")
						return nil
					}
					return check("static", invoke(runStatic(mod)))

				case "presubmit":
					fumpt := check("fmt", invoke(runFumpt(mod)))
					test := check("test", invoke(runTests(mod)))
					vet := check("vet", invoke(runVet(mod)))
					if fumpt != nil {
						return fumpt
					} else if test != nil {
						return test
					} else if vet != nil {
						return vet
					}

				case "fmt", "format":
					return check("fmt", invoke(runFumpt(mod)))

				default:
					return fmt.Errorf("subcommand %q not understood", arg)
				}
				return nil
			}()
			if err != nil {
				log.Printf("Error: %v", err)
				nerr++
			}
		}
	}
	if nerr > 0 {
		return fmt.Errorf("%d problems found", nerr)
	}
	return nil
}

// moduleRoot returns the module root for the current package.  If the go tool
// cannot find one, it delegates to rootDir.
func moduleRoot() (string, error) {
	mod, err := exec.Command("go", "env", "GOMOD").Output()
	if err == nil {
		path := strings.TrimSpace(string(mod))
		if path != "/dev/null" {
			return filepath.Dir(path), nil
		}
	}
	return rootDir()
}

// rootDir returns the root directory for the current repository.
func rootDir() (string, error) {
	data, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	return strings.TrimSpace(string(data)), err
}

func gocmd(dir string, args ...string) *exec.Cmd { return runcmd("go", dir, args...) }

func runcmd(bin, dir string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Dir = dir
	return cmd
}

func runTests(path string) *exec.Cmd { return gocmd(path, "test", "-race", "-cpu=1,2", "./...") }

func runTestsOnce(path string) *exec.Cmd { return gocmd(path, "test", "-race", "-cpu=2", "./...") }

func runVet(path string) *exec.Cmd { return gocmd(path, "vet", "./...") }

func runStatic(path string) *exec.Cmd { return runcmd("staticcheck", path, "./...") }

func runFumpt(path string) *exec.Cmd {
	const script = `find . -type f -name '*.go' -print0 \
| xargs -0 gofmt -l -s \
| grep .
if [ $? -eq 0 ] ; then
  echo "^ These files need formatting with go fmt"
  exit 1
fi
`

	cmd := exec.Command("/bin/sh", "-s", "--", "gofmt", "-l", "-s", "./...")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Dir = path
	return cmd
}

func invoke(cmd *exec.Cmd) error {
	fmt.Fprintf(out, "▷ \033[1;36m%s\033[0m\n", strings.Join(cmd.Args, " "))
	err := cmd.Run()
	switch t := err.(type) {
	case nil:
		fmt.Fprintln(out, "\033[50C\033[1;32mPASSED\033[0m")
	case *exec.Error:
		fmt.Fprintf(out, "\033[50C\033[1;33mSKIPPED\033[0m (%v)\n", t)
		return nil
	default:
		fmt.Fprintln(out, "\033[50C\033[1;31mFAILED\033[0m")
	}
	return err
}

func writeHook(path, subcommand, modFlag string) error {
	var insert string
	if modFlag != "" && modFlag != "auto" {
		insert = "-mod '" + modFlag + "' "
	}
	content := fmt.Sprintf(`#!/bin/sh
#
# Verify that the code is in a useful state before pushing.
git go %s%s
`, insert, subcommand)

	return os.WriteFile(path, []byte(content), 0755)
}

func installTools() error {
	for _, tool := range []string{
		"honnef.co/go/tools/cmd/staticcheck@2021.1.2",
		"golang.org/x/tools/cmd/goimports@latest",
	} {
		cmd := exec.Command("go", "install", tool)
		cmd.Dir = os.TempDir()
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		fmt.Fprintf(out, "[INSTALL] %s\n", tool)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("installing %q: %v", tool, err)
		}
	}
	return nil
}

func tagMode(tag string) string {
	if m := os.Getenv("GITGO_" + strings.ToUpper(tag)); m != "" {
		return m
	}
	return "error"
}

func check(tag string, err error) error {
	if _, ok := err.(*exec.ExitError); ok && tagMode(tag) == "warn" {
		fmt.Fprintf(out, "\t[NOTE] \033[1;33mIgnoring %[1]s failure "+
			"because %[1]s mode is \"warn\"\033[0m\n", tag)
		return nil
	}
	return err
}

func update(args *[]string, arg string) {
	trim := strings.TrimPrefix(arg, "-")
	drop := trim != arg
	for i, cur := range *args {
		if cur == trim {
			if drop {
				*args = append((*args)[:i], (*args)[i+1:]...)
			}
			return
		}
	}
	*args = append(*args, trim)
}

const presubmitConfig = `name: Go presubmit

on:
  push:
    branches:
      - default
  pull_request:
    types: [opened, reopened, synchronize]
  workflow_dispatch:

jobs:
  build:
    name: Go presubmit
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        go-version: ['1.17']
        os: ['ubuntu-latest']
    steps:
    - name: Install Go ${{ matrix.go-version }}
      uses: actions/setup-go@v2
      with:
        go-version: ${{ matrix.go-version }}
    - uses: actions/checkout@v2
    - uses: creachadair/go-presubmit-action@default
`

func installPresubmitWorkflow() error {
	path := filepath.Join(".github/workflows/go-presubmit.yml")
	if _, err := os.Stat(path); err == nil {
		return errors.New("presubmit workflow is already installed")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(presubmitConfig), 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed Go presubmit workflow at %q\n", path)
	fmt.Fprintln(out, "You must commit and push this file to enable the workflow")
	return nil
}

func findSubmodules(root, modFlag string) ([]string, error) {
	mods := []string{"."}
	if modFlag == "" || modFlag == "." {
		return mods, nil
	} else if modFlag != "auto" {
		return append(mods, strings.Split(modFlag, ",")...), nil
	}
	if err := filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if fi.Mode().IsRegular() && fi.Name() == "go.mod" {
			dir := filepath.Dir(path)
			if dir != root {
				mods = append(mods, strings.TrimPrefix(dir, root+"/"))
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return mods, nil
}

func isGo118() bool {
	bits, err := exec.Command("go", "version").Output()
	if err != nil {
		return false
	}

	fields := strings.Fields(string(bits))
	return len(fields) >= 3 && fields[2] == "go1.18"
}
