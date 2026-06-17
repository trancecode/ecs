# Go style guide

Conventions for this module. Adapted from the `nrg` style guide, trimmed to what
applies to a small, dependency-free library. Idiomatic Go applies everywhere these
rules are silent.

## Errors

* Never log and return; do one or the other.
* Never return a bare `err`; always add context. If there is genuinely none to add,
  say so in a comment (for example `// os.PathError already includes operation and filename`).
* Phrase messages as `<context>: <reason>`, where the context names the action being
  attempted and `<reason>` is generally the wrapped error:

      if err := w.load(path); err != nil {
          return fmt.Errorf("loading world from %q: %w", path, err)
      }

* Include context the caller lacks (loop indices, computed values, the operation), and
  omit context the caller already has. Use `%q` for strings that might be empty or dirty,
  and `%T` when reporting an unexpected type.
* Do not start messages with `failed to` or `error` (except when logging).

This library favors returning errors over panics for recoverable conditions. Reserve
`panic` for genuine programmer errors (invariant violations), not for input validation a
caller could reasonably hit.

## Naming

* Use camel case for acronyms: `EntityId`, not `EntityID`; `HttpClient`, not `HTTPClient`.
* Otherwise follow idiomatic Go naming.
* Let the package and receiver carry context; avoid stutter (`ecs.Get`, not `ecs.GetComponent`).

## Imports

Group imports, separated by blank lines, alphabetical within each group:

1. Standard library
2. Third-party
3. This module

## Enumerations

When defining an enumeration with `iota`, start with a `None`/`Invalid` zero value so an
uninitialized field is detectable:

    type Phase int

    const (
        PhaseNone Phase = iota // zero value: unset
        PhaseInput
        PhaseUpdate
        PhaseRender
    )

Validate against the zero value before use rather than relying on implicit defaults.

## Documentation

* Document every exported type, function, and struct field.
* Begin each doc comment with the name of the thing it documents.
* Explain purpose and corner cases, not line-by-line implementation. Be concise.
* Describe a function as a whole and how it uses its arguments; do not document each
  argument separately.
* Document exported struct fields on the line above the field, starting with the field
  name, and include units, ranges, or constraints where relevant:

      // Entities is the number of live entities in the world.
      Entities int

## Dos and don'ts

### Prefer early returns over `else`

Instead of:

    if ok {
        // ...
    } else {
        return
    }

prefer:

    if !ok {
        return
    }
    // ...

### Don't leave tombstone comments

When you remove logic, remove the comments about it too. Don't narrate what no longer
exists.

## Tooling

Before pushing, the following must be clean (CI enforces them):

    gofmt -l ecs
    go vet ./...
    go test ./... -race

`staticcheck ./...` is recommended locally if you have it installed.
