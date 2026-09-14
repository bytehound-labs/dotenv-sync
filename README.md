# dotenv-sync

[Source repository](https://github.com/bytehound-labs/dotenv-sync)

`dotenv-sync` is a cross-platform Go CLI for keeping a local `.env` file aligned
with a schema in `.env.example` while resolving provider-managed values through
a secret provider. Built-in providers currently include **Bitwarden** (via the
`rbw` CLI) and **KeePass** (via `keepassxc-cli`), with room for additional
providers over time.

## What people usually use it for

- Keep `.env.example` as the committed schema for required environment variables
- Fill local `.env` values from your configured secret provider
- Detect drift, malformed files, duplicates, and missing secrets before they
  break teammates or CI
- Bootstrap `.env.example` from an existing `.env`
- Use `ds push` for Bitwarden round-trip write-back workflows
- Use `ds scaffold` to bootstrap blank KeePass entries from `.env.example`

## Quick start

Most people just need a schema, a provider config, and `ds sync`. The examples
below assume `ds` is already installed and available; install/build options are
further down.

### 1. Define the schema in `.env.example`

```dotenv
# Application settings
DATABASE_URL=
JWT_SECRET=
PORT=8080
FILE=
```

Blank values are treated as provider-managed secrets unless the key is listed
under `local_keys` in `.envsync.yaml`. Literal values are treated as safe
defaults and copied into `.env`.

### 2. Point `ds` at your provider

Bitwarden example:

```yaml
provider: bitwarden
schema_file: .env.example
env_file: .env
item_name: my-app
mapping:
  DATABASE_URL: db_url
  JWT_SECRET: auth_jwt
local_keys:
  - FILE
```

KeePass example:

```yaml
provider: keepass
schema_file: .env.example
env_file: .env
keepass_database: /path/to/secrets.kdbx
keepass_group: dotenv
```

### 3. Check readiness and write `.env`

```bash
ds doctor
ds sync
```

That checks your provider prerequisites, then writes `.env` from
`.env.example`, copying safe defaults and resolving blank values from your
provider.

### 4. Inspect drift and unresolved values

```bash
ds diff
ds validate
ds missing
```

- `ds diff` previews real changes without writing files
- `ds validate` is the CI-friendly check for malformed files, drift, duplicates,
  and unresolved secrets
- `ds missing` lists unresolved schema keys only

Host-local values use the same blank schema assignment on every host, while each
host keeps its own value only in its ignored `.env`:

```yaml
local_keys:
  - FILE
```

```dotenv
# Host A
FILE=1
```

```dotenv
# Host B
FILE=2
```

Local values are never read from, written to, or compared through a provider.
When a local value is missing or blank, `ds sync`, `ds diff`, and `ds validate`
print a redacted warning telling the operator to set it in `.env`; the commands
still succeed and `ds sync` writes `FILE=` when needed. `ds missing` reports
provider-backed keys only.

## Command overview

| Command                       | What it is for                                                                      |
| ----------------------------- | ----------------------------------------------------------------------------------- |
| `ds sync`                     | Create or update `.env` from `.env.example` and your provider                       |
| `ds doctor`                   | Check config and provider readiness before syncing                                  |
| `ds diff`                     | Preview redacted drift without writing files                                        |
| `ds validate`                 | Fail on malformed files, drift, duplicates, or unresolved secrets                   |
| `ds missing`                  | List unresolved provider-backed keys                                                |
| `ds init`                     | Create `.env.example` from an existing `.env`                                       |
| `ds reverse`                  | Add new keys from `.env` back into `.env.example` as blanks                         |
| `ds push`                     | Upload `.env` back into Bitwarden (**Bitwarden-only**)                              |
| `ds scaffold`                 | Seed blank KeePass entries from `.env.example` for initial setup (**KeePass-only**) |
| `ds --version` / `ds version` | Show build and release metadata                                                     |

## Provider write-back support today

| Provider  | What `ds` can write today                                                                                                        | Recommended workflow                                                                                                              |
| --------- | -------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Bitwarden | `ds push` can write `.env` changes back to the provider                                                                          | Use `ds sync` to pull secrets and `ds push` when you want a round-trip workflow                                                   |
| KeePass   | `ds scaffold` can create missing blank entries from `.env.example`, but it does **not** update existing entry values from `.env` | Run `ds scaffold` once to bootstrap the vault structure, then edit secret values in KeePassXC and use `ds sync` to read them back |

## Configuration

`.envsync.yaml` is optional. Running `ds init` or `ds scaffold` with no config
present will walk you through first-run setup and write it for you.

### Value sources

Each schema assignment has one source:

| Source   | Schema value | Configuration              | Behavior                                                          |
| -------- | ------------ | -------------------------- | ----------------------------------------------------------------- |
| Static   | nonblank     | none                       | Copy the committed value into `.env`                              |
| Provider | blank        | not listed in `local_keys` | Resolve through Bitwarden or KeePass                              |
| Local    | blank        | listed in `local_keys`     | Preserve the current host's `.env` value and never use a provider |

Every `local_keys` entry must appear exactly once in `.env.example` with a blank
value and must not also appear in `mapping`.

### Bitwarden

```yaml
provider: bitwarden
schema_file: .env.example
env_file: .env
item_name: my-app
storage_mode: fields
mapping:
  DATABASE_URL: db_url
  JWT_SECRET: auth_jwt
local_keys:
  - FILE
```

### KeePass

```yaml
provider: keepass
schema_file: .env.example
env_file: .env
keepass_database: /path/to/secrets.kdbx
keepass_group: dotenv
```

`keepass_database` is the path to your `.kdbx` file (absolute or relative to the
project root). `keepass_group` is the group inside the vault that holds env var
entries — each entry's title is the key name and its password field is the
secret value. Defaults to `dotenv` if omitted.

`.kdbx` files are automatically ignored by the bundled `.gitignore` entry.

### Bitwarden-specific options

For Bitwarden, if `item_name` is omitted, `ds` derives it from the Git
repository root directory name and falls back to the current working directory
name when Git metadata is unavailable. By default, Bitwarden-managed keys
resolve as `rbw get <item_name> --field <ENV_VAR>`, and `mapping` overrides
only the field name inside that Bitwarden item. Keys listed under `local_keys`
bypass provider resolution.

Bitwarden `storage_mode` defaults to `fields` for backward-compatible reads
from the repo-scoped item fields. In `fields` mode, `ds push` can update
provider-managed keys that map to Bitwarden's built-in `password` field. Set
`storage_mode: note_json` to store the full repo env map in the item notes for
round-trip `push`/`sync` workflows. Local keys are omitted from the payload; if
an older payload contains one, the next `ds push` removes that stale provider
copy.

That makes shared aliases across repos possible with the default field-based
layout. For example, both repos below read and write the same Bitwarden value:

```yaml
# repo1/.envsync.yaml
item_name: shared-dev
mapping:
  DB_PASSWD: password
```

```yaml
# repo2/.envsync.yaml
item_name: shared-dev
mapping:
  PSWD: password
```

## Automate schema updates with Lefthook

If your team wants commits to keep a single `.env.example` in sync
automatically, a simple `lefthook` setup can run `ds reverse`, push provider
updates, and re-stage the schema before commit:

```yaml
pre-commit:
  parallel: true
  skip:
    - rebase
  commands:
    ds-sync:
      run: |
        ds reverse
        ds push
        git add .env.example
```

That example mirrors a real-world `lefthook` flow, but keeps it to one
`.env.example` so it stays easy to understand.

Use this pattern when:

- `.env.example` is your committed schema source of truth
- you want local env additions reflected back into the schema before commit
- you use Bitwarden write-back and want provider updates pushed automatically

If you use KeePass or another read-only flow, drop the `ds push` line and keep
just `ds reverse` plus `git add .env.example`.

For the smoothest Bitwarden write-back flow, use `storage_mode: note_json`.

## Refresh `.env` after pulling with Lefthook

The pre-commit hook above keeps `.env.example` and your provider in sync when
_you_ commit. A companion `post-merge` hook keeps your local `.env` in sync
when you pull in someone else's changes:

```yaml
post-merge:
  commands:
    ds-sync:
      run: |
        if rbw unlocked >/dev/null 2>&1; then
          ds sync
        else
          echo "Skipping ds sync: rbw is locked. Run 'rbw unlock' then 'ds sync' manually."
        fi
```

Use this pattern when:

- a teammate's commit might add new keys to `.env.example`
- a teammate's `ds push` might update a provider-managed value you don't have
  locally yet
- you want `git pull` (including a custom alias that wraps it) to leave
  `.env` current without a manual step afterward

Notes:

- `post-merge` fires on `git pull`, including fast-forward pulls, but not on
  a no-op "Already up to date" pull or on `git fetch` alone. A failing hook
  command never fails the pull itself — `post-merge` is a non-blocking git
  hook.
- `ds sync` errors if it can't reach the provider, even when `.env` is
  already fully up to date. For Bitwarden via `rbw`, checking
  `rbw unlocked` first (as above) turns a locked session into a quiet
  one-line skip instead of a hard error on every pull. Other providers or
  lock models may need a different readiness check, or none at all.
- Run `lefthook install` again after adding a hook type, like `post-merge`,
  that wasn't previously configured in `.lefthook.yml` — it needs to
  generate that hook's shim under `.git/hooks/`.

This is unrelated to this repository's own `post-merge` hook (see
[Keep `ds` up to date locally](#keep-ds-up-to-date-locally)), which
refreshes a locally installed `ds` binary after pulling changes to
`dotenv-sync` itself rather than syncing a consuming project's `.env`.

## Install and build

On Arch Linux, install the AUR package:

```bash
yay -S dotenv-sync-bin
```

That package installs the `ds` executable.

Without an AUR helper:

```bash
git clone https://aur.archlinux.org/dotenv-sync-bin.git
cd dotenv-sync-bin
makepkg -si
```

To build from source locally:

```bash
go build -o ./bin/ds ./cmd/ds
```

Local development builds report `dev` metadata by default. Release builds inject
their version, commit, and build time at build time instead of editing source
files.

## Command reference

### `ds sync`

```bash
ds sync
ds sync --dry-run
```

- Reads `.env.example` as the schema contract
- Resolves provider-managed blank entries and preserves configured `local_keys`
  from the current `.env`
- Preserves comment order and line endings when rewriting `.env`
- Produces `WRITTEN`, `UNCHANGED`, and `MISSING` output vocabulary for sync runs
- On successful writes, prints the changed keys before the final summary without
  exposing raw values
- Uses `CHECKED` summaries for dry-run previews
- Warns without failing when a configured local key is missing or blank, and
  writes a blank assignment when needed

### `ds push`

```bash
ds push --dry-run
ds push
```

- Bitwarden-only for now; other providers may add write support later
- Requires a Bitwarden write-capable setup; `note_json` is the simplest full
  round-trip mode
- Reads `.env` as the upload source and `.env.example` as schema context
- In `note_json`, writes a deterministic JSON payload into the repo-scoped
  Bitwarden item notes
- Excludes `local_keys` from both storage modes and removes stale local entries
  from an existing `note_json` payload
- In `fields`, updates provider-managed keys present in `.env` when they map to
  Bitwarden's built-in `password` field
- Never prints raw values; previews use redacted markers such as `[REDACTED]`
- Leaves `.env` and `.env.example` untouched

Field mode is useful for shared aliases across repos, but `rbw` cannot script
writes to arbitrary custom Bitwarden fields. If a repo maps pushed keys to
custom fields, `ds push` fails with actionable guidance to use `password` or
switch to `storage_mode: note_json`.

KeePass does **not** use `ds push` today. Its nearest equivalent is
`ds scaffold`, but that command only creates missing blank entries during
initial setup; it does not write current `.env` values back into KeePass.

### `ds diff`

```bash
ds diff
```

Prints only real changes using redacted markers such as `[RESOLVED]` and
`[STATIC]`. Missing or blank `local_keys` produce warning-only output and a
successful exit.

### `ds validate`

```bash
ds validate
```

Returns exit code `2` when drift, malformed input, duplicates, or unresolved
provider secrets are found. Missing or blank `local_keys` are warnings and do
not fail validation.

### `ds doctor`

```bash
ds doctor
```

Checks `.envsync.yaml` readability and provider readiness without printing any
secret values. Works for both Bitwarden and KeePass providers.

### `ds init`

```bash
ds init
ds init --dry-run
```

Creates `.env.example` from `.env`, blanking secret-like values and configured
`local_keys` while copying safe defaults.

If no `.envsync.yaml` exists and stdin is a terminal, `ds init` runs first-run
setup — asking which provider to use and writing the config file before
continuing. For KeePass, it verifies the database path and group exist before
writing anything.

If `.env` is malformed or contains duplicate keys, `ds init` returns a
validation error with actionable diagnostics instead of silently generating a
schema.

### `ds scaffold`

```bash
ds scaffold
ds scaffold --dry-run
```

Seeds a KeePass vault with blank entries for every provider-managed key in
`.env.example`. Configured `local_keys` are excluded. Existing entries are
skipped — never overwritten. Only supported when `provider: keepass`.

This is a bootstrap step, not a KeePass version of `ds push`: it creates the
entry structure, but it does not update existing KeePass entry values from
`.env`.

This is a **dev lead tool** for the recommended KeePass team workflow:

1. Design `.env.example` with blank values for all secrets
2. Run `ds scaffold` — creates stub entries in the vault
3. Open KeePassXC, fill in the real secret values
4. Run `ds sync` to verify `.env` populates correctly
5. Share the `.kdbx` file with team members via a secure channel
6. Team members run `ds sync` and are ready to go

### `ds missing`

```bash
ds missing
```

Lists only unresolved provider-managed schema keys and exits with code `2` when
any provider key is missing. Configured local keys are never reported here.

### `ds reverse`

```bash
ds reverse --dry-run
ds reverse
```

Adds keys found in `.env` but missing from `.env.example` as blank placeholders.
Configured local keys remain blank in the schema and retain their host-specific
value only in `.env`.

### `ds --version` and `ds version`

```bash
ds --version
ds version
```

- `ds --version` prints a concise version string such as `ds v0.4.0`
- `ds version` prints detailed metadata including version, commit, build time,
  and platform
- Local development builds fall back to `dev`, `none`, and `unknown`

## Exit codes

- `0`: success or no-op success
- `1`: operational failure
- `2`: validation, drift, or missing-value issue

## CI and releases

GitHub Actions validates every push, pull request, and manual dispatch through
`.github/workflows/go-tests.yml`. The workflow checks Go formatting and module
tidiness, runs `go vet ./...`, executes the full test suite on Ubuntu, macOS,
and Windows, runs race-enabled tests on Ubuntu, and builds the Linux, macOS,
and Windows release targets. The `Required checks` job is the stable aggregate
status for branch protection.

`.github/workflows/go-coverage.yml` generates a native Go coverage profile,
enforces the committed floor in `.github/coverage-floor.txt`, and uploads the
profile as a short-lived Actions artifact. The coverage floor is intentionally
kept in the repository so a coverage regression is visible in review without
requiring a third-party coverage service.

Security automation runs on pull requests and pushes to `main`, with scheduled
weekly scans:

- `.github/workflows/codeql.yml` runs CodeQL analysis for Go.
- `.github/workflows/gitleaks.yml` scans the complete Git history for leaked
  credentials and redacts findings.
- `.github/workflows/go-security.yml` runs the pinned `govulncheck` tool against
  reachable Go vulnerabilities.
- `.github/workflows/security-lint.yml` checks workflow syntax with `actionlint`
  and audits GitHub automation with `zizmor`.

`.github/dependabot.yml` requests weekly grouped security updates for Go
modules and GitHub Actions. Routine version-update pull requests remain
disabled so security fixes are not obscured by unrelated dependency churn.

Every push to `main` drives release automation automatically:

```bash
git switch main
git pull --ff-only
git push origin main
```

`.github/workflows/release.yml` runs on pushes to `main`, calculates the next
patch version, reruns `go test ./...`, builds versioned archives for Linux,
macOS, and Windows, bundles `README.md` and `LICENSE` into the release
archives, writes `ds_<version>_SHA256SUMS`, verifies the Linux reference
artifact with `ds --version`, generates a CycloneDX SBOM, publishes GitHub
OIDC provenance attestations, creates keyless Cosign signature bundles, and
then creates or refreshes the matching GitHub release. Release evidence is
published alongside the archives as `release-sbom.cdx.json`,
`release-assets.sha256`, `release-provenance.bundle.json`, and
`*.sigstore.json` files.

To verify a downloaded release, first check the archive checksum:

```bash
sha256sum --check ds_<version>_SHA256SUMS
```

GitHub provenance can be verified against the published attestation and the
release workflow identity:

```bash
gh attestation verify ds_<version>_linux_amd64.tar.gz \
  --repo bytehound-labs/dotenv-sync \
  --signer-workflow bytehound-labs/dotenv-sync/.github/workflows/release.yml
```

For offline verification, download `release-provenance.bundle.json` and pass
it with `--bundle` instead. The CycloneDX document can be inspected with any
CycloneDX-compatible tool or a JSON viewer. To verify a keyless Cosign bundle,
install Cosign and run:

```bash
cosign verify-blob \
  --bundle ds_<version>_linux_amd64.tar.gz.sigstore.json \
  --certificate-identity-regexp 'https://github.com/bytehound-labs/dotenv-sync/.github/workflows/release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ds_<version>_linux_amd64.tar.gz
```

If `AUR_SSH_PRIVATE_KEY` is configured, `.github/workflows/aur-publish.yml` is
invoked directly from `.github/workflows/release.yml` after the GitHub release
succeeds, and updates the `dotenv-sync-bin` AUR package from the published Linux
release artifacts. This direct downstream handoff is also the intended pattern
for future package-manager publishers. The AUR package installs the `ds`
executable even though the package name is `dotenv-sync-bin`.

If you need an AUR-only packaging fix without a new upstream release tag, rerun
`go run ./scripts/aurpkg` against the existing tag with a higher `--pkgrel`
value and push the updated AUR repo.

You can also run **Publish AUR package** manually from GitHub Actions with a
`release_tag` such as `v0.0.7` and a `pkgrel` such as `2`.

To enable AUR publishing from GitHub Actions:

1. Add an SSH key to your AUR account.
2. Save the matching private key as the `AUR_SSH_PRIVATE_KEY` repository secret.
3. Make sure the `dotenv-sync-bin` AUR repo exists at
   `ssh://aur@aur.archlinux.org/dotenv-sync-bin.git`.

You can monitor the latest release run with:

```bash
gh run list --workflow release.yml --limit 1
gh run watch <run-id>
```

The AUR publisher can be monitored separately with:

```bash
gh run list --workflow aur-publish.yml --limit 1
gh run watch <run-id>
```

## Keep `ds` up to date locally

If you keep `ds` in `~/.local/bin/ds`, the simplest update flow is:

```bash
git switch main
git pull --ff-only
./scripts/install-local.sh
```

By default `./scripts/install-local.sh` installs to `~/.local/bin/ds`. You can
override that with `--bin /custom/path/to/ds`.

If you use this repository's `lefthook` setup, `lefthook install` also enables
automatic local refreshes after `git commit` and `git merge` on the default
branch. Those hooks call `./scripts/install-local.sh --quiet` through
`./scripts/install-local-hook.sh`, and they intentionally skip non-default
branches so feature work does not overwrite your globally installed `ds`.

The script injects version metadata from your current checkout:

- exact release tag checkout: `v0.4.0`
- commits ahead of a release: `v0.4.0-3-gabc1234`
- no release tags yet: `dev-abc1234`

That means:

- if `main` is exactly on the latest release tag, `ds version` matches what a
  GitHub release installer sees
- if `main` is ahead of the latest release, your local build intentionally shows
  a newer git-derived version so it does not pretend to be the last published
  release

If you want the exact same version string as the latest GitHub release, build
from the release tag or install the GitHub release artifact.

## Development

Use the built-in session-scoped planning and execution workflow for
non-trivial changes. Keep transient plans and task lists outside the repository;
commit only durable project documentation and code.

Repository-specific contributor and coding-agent guidance lives in
[AGENTS.md](AGENTS.md).

```bash
gofmt -w ./path/to/changed.go
go test ./...
go test ./... -count=1 -covermode=atomic -coverprofile=coverage.out
go tool cover -func=coverage.out
go vet ./...
go test ./... -run TestContract
go test ./... -bench . -run '^$'
```

## Build a local versioned binary

Preview the next patch tag from the current reachable semver tags:

```bash
go run ./scripts/nextversion
```

Example outputs:

```text
v0.0.1
v0.4.3
```

`scripts/nextversion` is a local preview helper for the same automatic release
logic used in CI. By default it predicts the next **patch** release from the
current reachable semver tags.

Then build a local binary with that predicted release metadata:

```bash
VERSION=$(go run ./scripts/nextversion)
COMMIT=$(git rev-parse --short HEAD)
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

go build -o ./bin/ds \
  -ldflags "-X dotenv-sync/pkg/dotenvsync.Version=$VERSION -X dotenv-sync/pkg/dotenvsync.Commit=$COMMIT -X dotenv-sync/pkg/dotenvsync.BuildTime=$BUILD_TIME" \
  ./cmd/ds

./bin/ds --version
./bin/ds version
```

Local development builds without injected metadata fall back to `dev`, `none`,
and `unknown`.

To install straight into `~/.local/bin/ds` with Git-derived version metadata:

```bash
./scripts/install-local.sh
```

## Add `ds` to your `PATH`

You can always run the binary directly as `./bin/ds` (or `bin\ds.exe` on
Windows), but for a normal `ds ...` workflow you can either add the build
directory to your `PATH` or copy/symlink the binary into a directory that is
already on your `PATH`.

### POSIX shells (`bash`, `zsh`, `sh`)

Add your build directory to a shell startup file such as `~/.profile`,
`~/.bashrc`, or `~/.zshrc`:

```bash
export PATH="$PATH:/absolute/path/to/dotenv-sync/bin"
```

Reload the file or open a new shell:

```bash
source ~/.bashrc
```

### `fish`

```fish
fish_add_path /absolute/path/to/dotenv-sync/bin
```

### PowerShell

For the current session:

```powershell
$env:Path += ";C:\absolute\path\to\dotenv-sync\bin"
```

For a persistent install, add the same directory through your system PATH
settings or place `ds.exe` in a directory that is already on PATH.

### Alternative: copy or symlink the binary

Examples:

```bash
ln -s /absolute/path/to/dotenv-sync/bin/ds ~/.local/bin/ds
```

or:

```bash
install -Dm755 ./bin/ds ~/.local/bin/ds
```
