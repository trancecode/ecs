# Claude Instructions

## About This Document

**Scope:** These are universal guidelines that apply to all Claude AI work on this repository.

---

## Commit Configuration

* Author Name: `Claude Code`
* Author Email: `herve.quiroz+claude@gmail.com`
* Do NOT add a `Co-Authored-By:` line in commit messages

For branch naming, commit formatting, pre-commit checks, and PR conventions, see the `git-commit` skill.

## Clean-up changes

* When removing logic, don't leave comments about it. If things don't exist, we don't need to mention them

## Git Workflow

- If the task involves large or complex changes, ask the user whether they prefer a direct commit to `main` or a PR

## Repository-Specific Guidelines

This repository is a small, standalone Entity Component System (ECS) library in Go. It has no runtime dependencies (standard library only) and is consumed as a module by other projects, starting with `nrg`.

### Module layout

* The module is `github.com/trancecode/ecs`; the package lives in the `ecs/` subdirectory, so the import path is `github.com/trancecode/ecs/ecs`.
* Every Go source file in the package is named `ecs_<subsystem>.go` (for example `ecs_world.go`, `ecs_store.go`), with matching tests `ecs_<subsystem>_test.go`. The package doc lives in `ecs/ecs_doc.go`.
* Keep files small and single-responsibility; split by responsibility, not by technical layer.

### Go Version Consistency

`go.mod` is the single source of truth for this repository's Go version — it is the only place a Go version is written, and the workflows already derive their version from it via `go-version-file: go.mod`:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
    check-latest: true
```

Never hardcode a `go-version:` literal in a workflow. Because nothing else carries the version, a bump is a one-line change.

### Dependencies

* No third-party dependencies, and that is a deliberate property of a foundational library. A new dependency is a significant decision; if one is ever genuinely needed, call it out prominently in the PR description (name, version, purpose, justification, and whether it is test-only).

### Verification before pushing

Run these from the repository root; all must be clean:

	gofmt -l ecs            # must print nothing
	go vet ./...
	go build ./...
	go test ./... -race

Benchmarks live in `ecs/ecs_bench_test.go`:

	go test ./ecs/ -bench . -benchmem

### Conventions

* Follow `docs/styleguide.md` for Go style (errors, naming, imports, enumerations, documentation, and common dos and don'ts).
* The architecture and its deliberately-deferred future seams are in `docs/superpowers/specs/2026-06-17-ecs-framework-design.md`. Read it before changing the deferral/flush model, the storage model, or the public API. **API stability is the primary design driver**, because multiple repositories depend on this module; treat any change to an exported signature as a multi-repo migration.
* Document all exported types, functions, and struct fields, beginning each comment with the name of the thing it documents.
