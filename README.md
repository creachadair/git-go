# git-go

[![GoDoc](https://img.shields.io/static/v1?label=godoc&message=reference&color=blue)](https://pkg.go.dev/github.com/creachadair/git-go)

This repository provides a plugin for Git to handle some common Go development tasks.

Warning: This works, but is kind of a hack.

## Installation

```bash
go install github.com/creachadair/git-go@latest
```

## Basic Usage Examples

```bash
git go help      # display help
git go presubmit # run some basic presubmit checks
git go check     # run more detailed presubmit checks

git go install-hook  # install as a pre-push hook
```



