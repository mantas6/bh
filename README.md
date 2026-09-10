# bh

`bh` is a [`gh`](https://cli.github.com/)-style command line interface for
Bitbucket Cloud. It brings a familiar, terminal-first workflow to Bitbucket
pull requests: list, view, create, edit, check out, review, merge, decline, and
comment on pull requests without leaving your shell.

It targets the Bitbucket Cloud REST 2.0 API (`https://api.bitbucket.org/2.0`).
Bitbucket Data Center is not supported.

## Installation

Install the latest release with `go install`:

```sh
go install github.com/mantas6/bh/cmd/bh@latest
```

This places the `bh` binary in `$(go env GOPATH)/bin`; make sure that directory
is on your `PATH`.

Or build from source:

```sh
git clone https://github.com/mantas6/bh
cd bh
go build -o bh ./cmd/bh
```

## Authentication

`bh` authenticates with a Bitbucket Cloud **API token**. App passwords were
removed by Atlassian in July 2026 and are not supported.

1. Create an API token at
   <https://id.atlassian.com/manage-profile/security/api-tokens> with these
   scopes:
   - `read:user:bitbucket`
   - `read:repository:bitbucket`
   - `read:pullrequest:bitbucket`
   - `write:pullrequest:bitbucket` (for mutating commands such as create,
     merge, approve, and comment)

2. Log in:

   ```sh
   bh auth login
   ```

   You are prompted to paste the token. Leaving the email blank sends the token
   as a `Bearer` token. Providing your Atlassian account email switches to
   `Basic` authentication (`email:token`), which some setups require:

   ```sh
   bh auth login --email you@example.com
   ```

   You can also pipe the token from standard input:

   ```sh
   bh auth login --with-token < token.txt
   ```

The token is validated against `GET /2.0/user` and stored in
`~/.config/bh/hosts.yml` with `0600` permissions.

Set `BH_TOKEN` in the environment to override the stored token (useful in CI).
When set, it takes precedence over `hosts.yml`.

## Repository resolution

Commands that operate on a repository resolve it in this order:

1. The `-R`/`--repo` flag (`ws/repo` or a full Bitbucket URL).
2. The `BH_REPO` environment variable.
3. The `upstream` git remote.
4. The `origin` git remote.
5. The first git remote that points at Bitbucket Cloud.

```sh
bh pr list -R myworkspace/myrepo
```

The following remote URL forms are recognized:

- `https://bitbucket.org/ws/repo.git`
- `https://user@bitbucket.org/ws/repo.git`
- `git@bitbucket.org:ws/repo.git`
- `ssh://git@bitbucket.org/ws/repo.git`
- `ssh://git@altssh.bitbucket.org:443/ws/repo.git`

## Command reference

| Command | Description |
| --- | --- |
| `bh auth login` | Log in to Bitbucket |
| `bh auth logout` | Log out of Bitbucket |
| `bh auth status` | View authentication status |
| `bh auth token` | Print the auth token bh is configured to use |
| `bh pr list` | List pull requests |
| `bh pr view` | View a pull request |
| `bh pr create` | Create a pull request |
| `bh pr edit` | Edit a pull request |
| `bh pr checkout` | Check out a pull request in git |
| `bh pr merge` | Merge a pull request |
| `bh pr approve` | Approve a pull request |
| `bh pr review` | Add a review to a pull request |
| `bh pr decline` | Decline a pull request (alias: `close`) |
| `bh pr comment list` | List comments on a pull request |
| `bh pr comment add` | Add a comment to a pull request |
| `bh pr comment reply` | Reply to a pull request comment |
| `bh pr comment delete` | Delete a pull request comment |
| `bh pr comment resolve` | Resolve a pull request comment thread |
| `bh pr comment reopen` | Reopen (unresolve) a pull request comment thread |

A pull request argument accepts a number (`123`), a Bitbucket pull request URL,
or may be omitted to use the pull request for the current branch.

Run `bh <command> --help` for the full flag list of any command.

## Usage examples

```sh
# List open pull requests
bh pr list

# List everything by a given author
bh pr list --state all --author @me

# View the pull request for the current branch
bh pr view

# Create a pull request, filling title and body from commits
bh pr create --fill --base main --reviewer alice,bob

# Check out a pull request locally
bh pr checkout 123

# Approve and merge
bh pr approve 123
bh pr merge 123 --squash --delete-branch

# Comment on a pull request
bh pr comment add 123 --body "Looks good to me"

# Add an inline comment on a specific line
bh pr comment add 123 --path main.go --line 42 --body "Rename this"
```

## Configuration

Configuration (including the stored API token) lives in `hosts.yml` inside the
config directory, resolved as:

1. `$BH_CONFIG_DIR`
2. `$XDG_CONFIG_HOME/bh`
3. `~/.config/bh`

The directory is created with `0700` permissions and `hosts.yml` with `0600`.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `BH_TOKEN` | API token override; takes precedence over the stored token |
| `BH_REPO` | Default repository (`ws/repo`) when no `-R` flag is given |
| `BH_CONFIG_DIR` | Override the configuration directory |
| `XDG_CONFIG_HOME` | Base config directory when `BH_CONFIG_DIR` is unset |
| `BROWSER` | Command used to open URLs for `--web` |
| `NO_COLOR` | Disable ANSI color output when set |

Color is emitted only when standard output is a terminal, `NO_COLOR` is unset,
and `TERM` is not `dumb`.

## Not supported / roadmap

The following are intentionally out of scope for now:

- Bitbucket Data Center (server) support
- Storing credentials in an OS keyring
- `--json <fields>` field selection (only a boolean `--json` dump is available)
- `pr diff`, `pr checks`, and `pr status`
- A bundled CI workflow
</content>
</invoke>
