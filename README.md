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
make build   # static binary ./bh with the version from `git describe`
./bh version
```

A plain `go build -o bh ./cmd/bh` also works; the version then falls back to
the module build info (or `dev`). Other targets: `make test`, `make race`,
`make lint` (requires `golangci-lint` v2), `make vet`, `make fmt-check` and
`make clean`.

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
   - `read:workspace:bitbucket` (to resolve reviewer names for
     `pr create --reviewer` and `pr edit --add-reviewer`; not needed when
     reviewers are passed as `{uuid}` or account ID)

   When a token lacks a scope, errors name it, e.g.
   `... (HTTP 403): required scopes: read:workspace:bitbucket; granted: ...`.

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
When set, it takes precedence over `hosts.yml`, and the email stored there is
ignored: set `BH_EMAIL` alongside `BH_TOKEN` to use Basic auth, or leave it
unset to use Bearer auth. `bh auth status` shows which source is in use.

## Repository resolution

Commands that operate on a repository resolve it in this order:

1. The `-R`/`--repo` flag (`ws/repo`, `bitbucket.org/ws/repo`, or a Bitbucket
   web/clone URL such as `git@bitbucket.org:ws/repo.git`). This works outside
   a git checkout.
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
| `bh pr list` | List pull requests (alias: `ls`) |
| `bh pr view` | View a pull request |
| `bh pr diff` | View changes in a pull request (`--name-only`, `--stat`, `--web`) |
| `bh pr commits` | List the commits of a pull request |
| `bh pr checks` | Show build status checks of a pull request (`--watch`) |
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
| `bh version` | Show the bh version (also `bh --version`) |

`pr list`, `pr view`, `pr commits`, `pr checks` and `pr comment list` take
`--json[=fields]` and `--jq <expression>`; see [JSON output](#json-output).

A pull request argument accepts a number (`123` or `#123`), a Bitbucket pull
request URL, or a branch name (the open pull request whose source branch it
is). For most commands it may be omitted to use the pull request for the
current branch; `bh pr checkout` and `bh pr comment reply|delete|resolve|reopen`
require it.

Run `bh <command> --help` for the full flag list of any command.

An unknown subcommand is a usage error: bh exits non-zero, suggests close
matches, and for commands that do not exist (such as `bh pr comments` or
`bh pr files`) points at what to use instead:

```console
$ bh pr comments 123
bh: unknown command "comments" for "bh pr"
Did you mean "comment" or "commits"?
To list comments, run: bh pr comment list [<pr>], or bh pr view [<pr>] --comments

Run 'bh pr --help' for usage.
```

That layout is used when standard error is a terminal. When it is not (piped,
redirected, or captured by a script or AI agent), every error is printed as a
single line, with the hint and usage pointer folded in, so `2>&1 | tail -1`
still shows the whole message:

```console
$ bh pr comments 123 2>&1 | cat
bh: unknown command "comments" for "bh pr"; Did you mean "comment" or "commits"?; To list comments, run: bh pr comment list [<pr>], or bh pr view [<pr>] --comments (run 'bh pr --help' for usage)
```

`bh pr list` prints one row per pull request: number, title,
`source → destination` branches, author, state and last update. On a terminal the update time is relative; otherwise the fields
are tab-separated and the time is an RFC 3339 timestamp. `--base` (`-B`) and
`--head` (`-H`) filter by destination and source branch, and combine with
`--state`, `--author` and `--search`. When more pull requests match than
`--limit` (`-L`, default 30) allows, a notice such as
`Showing 30 of 142 results; use --limit to see more` is printed on standard
error (the total is included when Bitbucket reports it); `--json` output never
gets the notice.

```console
$ bh pr list --base dev
#934  Add login form   ft/login → dev   ada   OPEN  about 3 hours ago
#930  Fix footer       ft/footer → dev  bob   OPEN  2 days ago
```

`bh pr view` prints a summary header followed by the description. On
Bitbucket anyone with access can approve, not only requested reviewers, so the
approval count and the `Approved by:` / `Changes requested by:` lines cover
every participant; `Reviewers:` lists only requested reviewers (`none` when
there are none). The two participant lines are omitted when empty. The header
reads `merged` / `declined` instead of `wants to merge` for closed pull
requests. The line after it shows the source (`Head:`) and destination
(`Base:`) commits as short hashes, the base branch, and the last update time as
an RFC 3339 timestamp in the local time zone (followed by the relative time on
a terminal); a hash Bitbucket did not report is left out. When builds have reported on the pull request, a `Checks:` line
counts their latest states (see `bh pr checks`); it is left out when there are
none or the statuses cannot be fetched. `--json` prints only the pull request
object and does not fetch build statuses.

```console
$ bh pr view 123
Add feature #123
OPEN • ada wants to merge feature into main • 2 approvals • 3 comments
Head: 0123456789ab · Base: main fedcba987654 · Updated: 2026-09-10T11:00:00+02:00
Reviewers: bob (approved ✓), cara (changes requested)
Approved by: bob, dan
Changes requested by: cara
Checks: 2 passed, 1 failed

This does things.

3 comments · bh pr comment list 123
View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123
```

When the pull request has comments, a `N comments · bh pr comment list <pr>`
hint is printed after the description, just above the URL line (which stays
last). Pass `--comments` (`-c`) to print the comment threads instead, in the
same format as `bh pr comment list`: a `#id author · time · path:line
[resolved]` header per comment, indented bodies, and replies nested under
their parent. `--unresolved` shows only unresolved threads and implies
`--comments`. `--comments` cannot be combined with `--json` or `--web`; use
`bh pr comment list --json` for comments as JSON.

```console
$ bh pr view 123 --comments
Add feature #123
OPEN • ada wants to merge feature into main • 2 approvals • 3 comments
Head: 0123456789ab · Base: main fedcba987654 · Updated: 2026-09-10T11:00:00+02:00
Reviewers: bob (approved ✓), cara (changes requested)
Approved by: bob, dan
Changes requested by: cara

This does things.

Comments:
#1 ada · about 2 hours ago · main.go:42 [resolved]
  Rename this

  #2 bob · 30 minutes ago
    Done

#3 cara · 5 minutes ago
  Why?

View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123
```

`bh pr diff` prints the pull request's unified diff as Bitbucket renders it
(colored on a terminal). `--name-only` prints one changed path per line, and
`--stat` prints one line per file with its status (`added`, `modified`,
`removed`, `renamed`, ...) and line counts; a renamed file is shown as
`old → new`. When standard output is not a terminal the `--stat` fields are
tab-separated and the summary line is left out:

```console
$ bh pr diff 123 --stat
main.go                    modified  +3   -1
new.go                     added     +10  -0
old/name.go → new/name.go  renamed   +0   -0
3 files changed, 13 insertions(+), 1 deletion(-)
```

`bh pr commits` lists the commits newest first: short hash, author, date and
the first line of the message. Dates are relative on a terminal and RFC 3339
timestamps otherwise (fields are then tab-separated); `--json` prints the full
commit objects.

`bh pr checks` lists the build statuses (CI checks) reported on a pull
request: name, state (`SUCCESSFUL`, `FAILED`, `INPROGRESS` or `STOPPED`), last
update and URL, failed checks first. When a build reported several times only
its latest status is shown. On a terminal a summary line comes first and times
are relative; otherwise fields are tab-separated and times are RFC 3339
timestamps. The command exits with status 1 when any check failed (also with
`--json`), so it can gate scripts. `--watch` polls every 10 seconds until no
check is in progress, then prints the final result (redrawing the screen on a
terminal, printing only the final result otherwise).

```console
$ bh pr checks 123
Checks: 2 passed, 1 failed

Lint    FAILED      30 minutes ago     https://ci.example.com/lint/42
Build   SUCCESSFUL  about an hour ago  https://ci.example.com/build/42
Test    SUCCESSFUL  about an hour ago  https://ci.example.com/test/42
```

### JSON output

`--json` (on `pr list`, `pr view`, `pr commits`, `pr checks` and
`pr comment list`) prints Bitbucket's objects with the API's field names.
Fields Bitbucket only reports in some contexts are left out when absent rather
than printed as `""`, `false` or `null`: for example the repository nested in a
pull request's `source`/`destination` carries `uuid`, `name`, `full_name` and
`links`, but no `slug`, `is_private`, `mainbranch`, `owner`, `workspace` or
`parent`. Lists are always arrays, `[]` when empty, never `null`: the command
results themselves, a pull request's `reviewers` and `participants` (which
`pr list --json` also includes), and a commit's `parents`. Top-level pull
request fields such as `merge_commit` and `closed_by` stay present and are
`null` until set.

`bh pr comment list --json` prints the comments of the listed threads in the
API's order (deleted comments are left out, and `--unresolved` / `--limit`
apply as in the text view). Each comment keeps the raw API fields (`parent`,
`inline` and `resolution` are omitted when unset) and adds three derived
fields:

| Field | Meaning |
| --- | --- |
| `resolved` | `true` when the comment's thread is resolved. Bitbucket records the resolution on the thread's top-level comment only; replies inherit it here. |
| `thread_id` | Id of the thread's top-level comment (the comment's own id for a top-level comment). Resolve or reopen the thread with this id. |
| `is_reply` | `true` when the comment is nested under another comment, i.e. `thread_id` differs from `id`. A reply whose parent was deleted is shown as its own thread. |

```sh
# Ids of the unresolved threads
bh pr comment list 123 --json --jq '[.[] | select((.is_reply or .resolved) | not) | .id]'
```

#### Selecting fields: `--json <fields>`

`--json` alone prints every field. Give it a comma-separated list to keep only
those fields, in the order given; dotted paths reach into nested objects and
apply to every element of a list (`reviewers.nickname`). For list commands
the selection applies to each item. Selected fields that are absent from an
object (see above) are left out; a path through a `null` value keeps the
`null`. An unknown field is an error that lists the available fields.

```sh
bh pr view 123 --json=id,title,state,source.branch.name
bh pr list --json id,title,author.nickname
bh pr comment list 123 --json id,thread_id,resolved,inline.path
```

`--json=<fields>` always works. The space-separated form `--json <fields>` is
recognized when the next argument is a field list for that command (all its
items are fields, it contains a comma, or it starts with a known field name);
otherwise it is taken as the command's argument, so `bh pr view --json 123`
still means "pull request 123 as JSON". Put a pull request argument that
could be mistaken for a field name (such as a branch called `state`) before
`--json`, or use `--json=<fields>`.

#### Filtering with `--jq`

`--jq <expression>` (`-q`) filters the JSON with a built-in
[jq](https://jqlang.org/manual/) implementation
([gojq](https://github.com/itchyny/gojq)), so `jq` need not be installed. It
requires `--json` and runs after any field selection. String results are
printed raw (like `jq -r`), other values as JSON, one result per line, compact
when piped and indented on a terminal. An invalid expression is a usage error;
an error while evaluating it is reported after the results printed so far.

```sh
bh pr view 123 --json --jq .source.branch.name
bh pr list --json --jq '.[] | "\(.id)\t\(.title)"'
bh pr checks 123 --json -q '.[] | select(.state == "FAILED") | .url'
```

## Usage examples

```sh
# List open pull requests
bh pr list

# List everything by a given author
bh pr list --state all --author @me

# List pull requests into dev, or the one(s) from a given branch
bh pr list --base dev
bh pr list --state all --head ft/login

# View the pull request for the current branch
bh pr view

# View a pull request with its unresolved comment threads
bh pr view 123 --unresolved

# Create a pull request, filling title and body from commits
bh pr create --fill --base main --reviewer alice,bob

# Add a reviewer by {uuid} or account ID (skips the workspace member lookup)
bh pr edit 123 --add-reviewer '{a1b2c3d4-e5f6-7890-abcd-ef1234567890}'

# Show the diff of a pull request, or just the changed files / line counts
bh pr diff 123
bh pr diff 123 --name-only
bh pr diff 123 --stat

# List the commits of a pull request (newest first)
bh pr commits 123

# Show build statuses; wait until running builds finish (exit 1 if any failed)
bh pr checks 123
bh pr checks 123 --watch

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

A relative `XDG_CONFIG_HOME` is ignored. If none of these can be determined
(for example `HOME` is unset), bh exits with an error rather than guessing.

The directory is created with `0700` permissions. `hosts.yml` is written
atomically and always saved with `0600` permissions, tightening any looser
permissions on an existing file.

## Environment variables

| Variable | Purpose |
| --- | --- |
| `BH_TOKEN` | API token override; takes precedence over the stored token and email |
| `BH_EMAIL` | Atlassian email paired with `BH_TOKEN` for Basic auth (ignored without `BH_TOKEN`) |
| `BH_REPO` | Default repository (`ws/repo`) when no `-R` flag is given |
| `BH_CONFIG_DIR` | Override the configuration directory |
| `XDG_CONFIG_HOME` | Base config directory when `BH_CONFIG_DIR` is unset |
| `BROWSER` | Command used to open URLs for `--web` (see below) |
| `NO_COLOR` | Disable ANSI color output when set |

`BROWSER` is split into words with shell-style quoting, so paths containing
spaces can be single- or double-quoted (or backslash-escaped). If any word
contains `%s` it is replaced with the URL; otherwise the URL is appended as the
last argument:

```sh
export BROWSER="'/opt/My Browser/browser' --new-tab"
export BROWSER='firefox --new-window %s'
```

Color is emitted only when standard output is a terminal, `NO_COLOR` is unset,
and `TERM` is not `dumb`.

## Not supported / roadmap

The following are intentionally out of scope for now:

- Bitbucket Data Center (server) support
- Storing credentials in an OS keyring
- `pr status`
- A bundled CI workflow
