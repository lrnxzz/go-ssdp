# Contributing

You need Go at the version in [`go.mod`](go.mod), Docker for the integration tests and
[golangci-lint](https://golangci-lint.run) v2.

Changes reach `main` through pull requests from short-lived branches. Commit messages follow
[Conventional Commits](https://www.conventionalcommits.org), which GoReleaser turns into the changelog
when a `vX.Y.Z` tag is pushed.

## Checks

The CI runs the same scripts you can run locally:

```sh
scripts/lint.sh
scripts/test.sh
scripts/fuzz.sh
```

`scripts/coverage.sh` shows what the tests leave uncovered; the CI fails below the threshold in
[`.testcoverage.yml`](.testcoverage.yml).

## Tests

Unit tests sit next to the code and cover pure logic, and the transport tests run real multicast that
never leaves the machine. Integration tests live in [`ssdp/test`](ssdp/test): every run starts real SSDP
implementations in Docker, and nothing recorded or faked stands in for them. Testing against another
implementation takes an image under `scripts/containers`, an `InitializeX` in
[`ssdp/test/root_test.go`](ssdp/test/root_test.go) and a `Test_InitializeX` that proves it starts.

## Code style

Code is formatted with gofumpt and goimports and has to pass the linter.

Types are built with `NewX()` constructors, and behavior is exposed as an interface backed by an
unexported struct, the way `NewControlPoint()` returns a `ControlPoint`. Names are whole words, with
acronyms written as words too, like `Uuid` and `BootId`. Composite literals that hold anything put one
element per line, types are always declared at package level, and code lives where it is used instead
of in small helpers.

Comments are technical and explain a decision taken from a detail of the protocol, citing the section
of the specification it comes from, like `UDA 1.1 §1.3.2`. Code that reads plainly gets no comment.
