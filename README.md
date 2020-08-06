# git-go

http://godoc.org/github.com/creachadair/git-go

This repository provides a plugin for Git to handle some common Go development tasks.

## Installation

```
(cd /tmp; go get github.com/creachadair/git-go)
```

## Basic Usage Examples

```bash
git go help      # display help
git go presubmit # run some basic presubmit checks
git go check     # run more detailed presubmit checks

git go install-hook  # install as a pre-push hook
```



