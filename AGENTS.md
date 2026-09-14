# dotenv-sync contributor guidance

## Purpose and scope

`dotenv-sync` is a cross-platform Go CLI that keeps a local `.env` file aligned
with a committed `.env.example` schema. It resolves provider-managed values
through Bitwarden or KeePass while protecting secret values from source control
and command output.

Treat `README.md` as the canonical user-facing documentation. This file defines
repository-specific implementation guidance for coding agents and contributors.

## Working approach

- Read the relevant implementation, tests, and README sections before changing
  behavior.
- Keep changes focused, reviewable, and compatible with existing command
  contracts. Do not add transient plans, task lists, or generated agent
  artifacts to the repository.
- Preserve unrelated working-tree changes. Do not rewrite, reformat, or
  regenerate files outside the requested scope.
- Prefer existing patterns and helpers over new abstractions. Add dependencies
  only when they solve a demonstrated need.

## Pull request completion

- Own each pull request through completion unless the user explicitly asks to
  stop before merging.
- After pushing a pull request, monitor every relevant GitHub Actions check
  until it finishes. Do not merge while a required check is pending or failing.
- When a check fails, inspect its logs, fix the root cause, run the relevant
  local validation, commit the fix, push it, and resume monitoring. Repeat this
  loop until all required checks pass.
- Once checks pass and the pull request is mergeable, merge it with the GitHub
  CLI, then verify that the merge completed successfully and the working branch
  reflects the new base state.
- For a sequence of focused pull requests, merge the current pull request
  before starting work on the next one.

## Architecture

| Area                | Responsibility                                                  |
| ------------------- | --------------------------------------------------------------- |
| `cmd/ds`            | Process entry point                                             |
| `internal/cli`      | Cobra commands, flags, streams, and user-facing rendering       |
| `internal/config`   | `.envsync.yaml` loading, normalization, and path resolution     |
| `internal/envfile`  | Parsing, merging, and faithful `.env` document writing          |
| `internal/sync`     | Command planning and provider-independent synchronization logic |
| `internal/provider` | Provider interfaces and provider-specific adapters              |
| `internal/report`   | Error categories, exit codes, summaries, and redaction          |
| `internal/fs`       | Atomic file writes                                              |
| `test/contract`     | Stable CLI and workflow behavior                                |
| `test/integration`  | End-to-end command and script coverage                          |

Keep the core sync engine provider-independent. New provider capabilities belong
behind the shared provider interfaces, with provider-specific command execution
contained in the appropriate adapter.

## Behavior that must be preserved

- `.env.example` is the committed schema. Literal schema values are safe static
  defaults; blank schema values are provider-managed unless their keys are
  listed under `local_keys` in `.envsync.yaml`. Local values belong only in the
  ignored `.env`, and missing local values are warning-only. Source
  classification is a user-facing contract, so centralize source types instead
  of scattering special-case blank-value checks.
- Preserve schema ordering, comments, inline comments, line endings, and
  no-op file stability. Use the existing document and atomic-write helpers
  rather than reconstructing dotenv files ad hoc.
- Keep command output safe to share: never print actual values from `.env`,
  providers, fixtures, errors, snapshots, logs, or pull requests. Use the
  repository's redacted report vocabulary.
- Maintain the documented exit-code distinction: operational failures return
  `1`; malformed input, drift, duplicates, and unresolved values return `2`.
- `ds push` is Bitwarden-only. KeePass `ds scaffold` creates only missing blank
  entry structure, excludes `local_keys`, and must never overwrite existing
  KeePass values. Provider write-back must never include local values.
- Preserve Linux, macOS, and Windows behavior. Isolate genuinely
  platform-specific code and test it through the existing portable harnesses.

## Implementation and tests

- Add focused unit coverage beside the code that changes. Update contract or
  integration tests when command output, exit codes, workflow behavior, or
  cross-package interactions change.
- Use temporary directories and the existing fake provider/CLI helpers in
  tests. Never call a real Bitwarden or KeePass vault from tests.
- Format changed Go files with `gofmt`. If dependencies change, run
  `go mod tidy` and include the resulting `go.mod` and `go.sum` changes.
- Run the smallest relevant tests while iterating, then run:

  ```bash
  go test ./...
  go vet ./...
  ```

## Documentation and automation

- Update `README.md` and command help whenever behavior, configuration, output,
  providers, setup, or release verification changes.
- Keep GitHub Actions portable and deterministic. Preserve the automatic
  patch-release-on-`main` contract, cross-platform archives, checksums, and
  AUR handoff unless the change intentionally revises those behaviors.
- Do not add secrets, credentials, real vault output, `.env` files, `.kdbx`
  files, build artifacts, or local tool state to version control.
