# dotenv-sync development guidance

## Workflow

- Use the built-in session-scoped planning and execution workflow for
  non-trivial changes.
- Keep transient plans, task lists, and investigation notes outside the
  repository. Commit only durable project documentation.
- Work in small, reviewable changes and keep each pull request focused.

## Implementation

- Read the relevant code, tests, and README before editing.
- Preserve the normal `.env` and `.env.example` workflow, including comments,
  ordering, line endings, atomic writes, and redacted operator output.
- Keep provider adapters behind the shared provider interfaces and avoid
  provider-specific behavior in the core sync engine unless the configuration
  explicitly requires it.
- Never place secrets in source, tests, fixtures, logs, pull requests, or
  committed documentation.
- Update `README.md` and command help whenever user-visible behavior changes.

## Validation

- Format changed Go files with `gofmt`.
- Run `go test ./...` and `go vet ./...` before opening a pull request.
- Add focused unit, contract, or integration coverage for behavior changes.
- Keep cross-platform behavior in mind; the CLI supports Linux, macOS, and
  Windows, while provider-specific tools may have narrower platform support.

## Releases

- Release metadata is derived from Git tags and build-time linker flags.
- Keep release artifacts reproducible and preserve the automatic release and
  downstream packaging workflow unless a change explicitly revises that
  contract.
