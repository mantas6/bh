package api

import (
	"bytes"
	"encoding/json"
	"time"
)

// JSON encoding conventions for the types in this package, which bh prints
// with --json:
//
//   - Fields Bitbucket only sends in some contexts (for example the
//     repository's slug, owner or main branch, which are absent from the
//     repository reference nested in a pull request) are omitted when unset
//     instead of being printed as "", false or null.
//   - List fields that are part of an object's shape (pull request reviewers
//     and participants, commit parents) always encode as an array, [] when
//     empty, never as null.

// Link is a single hypermedia link.
type Link struct {
	Href string `json:"href"`
}

// CloneLink is a named clone URL (https/ssh).
type CloneLink struct {
	Name string `json:"name"`
	Href string `json:"href"`
}

// Links is the common `links` object.
type Links struct {
	HTML  Link        `json:"html,omitzero"`
	Self  Link        `json:"self,omitzero"`
	Clone []CloneLink `json:"clone,omitempty"`
}

// User is a Bitbucket account.
type User struct {
	UUID        string `json:"uuid,omitempty"`
	AccountID   string `json:"account_id,omitempty"`
	Nickname    string `json:"nickname,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Type        string `json:"type,omitempty"`
	Links       Links  `json:"links,omitzero"`
}

// Workspace is a Bitbucket workspace.
type Workspace struct {
	UUID string `json:"uuid,omitempty"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name,omitempty"`
}

// Branch is a repository branch / merge configuration. The merge strategy
// fields are only reported for some branch references (such as a pull
// request's destination).
type Branch struct {
	Name                 string   `json:"name"`
	DefaultMergeStrategy string   `json:"default_merge_strategy,omitempty"`
	MergeStrategies      []string `json:"merge_strategies,omitempty"`
}

// Repository is a Bitbucket repository. The repository reference nested in
// a pull request only carries uuid, name, full_name and links; the other
// fields are set when the repository itself is fetched. IsPrivate is a
// pointer so an unreported visibility is not mistaken for "public".
type Repository struct {
	UUID       string      `json:"uuid,omitempty"`
	Name       string      `json:"name,omitempty"`
	FullName   string      `json:"full_name,omitempty"`
	Slug       string      `json:"slug,omitempty"`
	IsPrivate  *bool       `json:"is_private,omitempty"`
	MainBranch *Branch     `json:"mainbranch,omitempty"`
	Links      Links       `json:"links,omitzero"`
	Owner      *User       `json:"owner,omitempty"`
	Workspace  *Workspace  `json:"workspace,omitempty"`
	Parent     *Repository `json:"parent,omitempty"`
}

// Commit is a minimal commit reference.
type Commit struct {
	Hash string `json:"hash"`
}

// CommitAuthor is the author of a commit: the raw git author
// ("Name <email>") and, when Bitbucket could link it, the account.
type CommitAuthor struct {
	Raw  string `json:"raw"`
	User *User  `json:"user,omitempty"`
}

// PRCommit is a commit listed on a pull request.
type PRCommit struct {
	Hash    string       `json:"hash"`
	Date    time.Time    `json:"date"`
	Message string       `json:"message"`
	Author  CommitAuthor `json:"author"`
	Parents []Commit     `json:"parents"`
	Links   Links        `json:"links,omitzero"`
}

// MarshalJSON encodes c with Parents as [] rather than null when empty.
func (c PRCommit) MarshalJSON() ([]byte, error) {
	type plain PRCommit
	p := plain(c)
	p.Parents = nonNil(p.Parents)
	return marshalNoEscape(p)
}

// DiffStatFile is one side (old or new) of a diffstat entry.
type DiffStatFile struct {
	Path string `json:"path"`
}

// DiffStat is the per-file change summary of a pull request diff. Status is
// "added", "removed", "modified", "renamed" or a conflict state; Old is nil
// for added files and New is nil for removed ones.
type DiffStat struct {
	Status       string        `json:"status"`
	LinesAdded   int           `json:"lines_added"`
	LinesRemoved int           `json:"lines_removed"`
	Old          *DiffStatFile `json:"old"`
	New          *DiffStatFile `json:"new"`
}

// Build status states reported by CommitStatus.State.
const (
	StatusSuccessful = "SUCCESSFUL"
	StatusFailed     = "FAILED"
	StatusInProgress = "INPROGRESS"
	StatusStopped    = "STOPPED"
)

// CommitStatus is a build status (CI check) reported on a commit, as listed
// for a pull request. State is one of StatusSuccessful, StatusFailed,
// StatusInProgress or StatusStopped; Key identifies the reporting build and
// stays the same across reruns.
type CommitStatus struct {
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	State       string    `json:"state"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	Refname     string    `json:"refname,omitempty"`
	CreatedOn   time.Time `json:"created_on"`
	UpdatedOn   time.Time `json:"updated_on"`
}

// PRRef is a pull request source/destination endpoint.
type PRRef struct {
	Branch     Branch      `json:"branch"`
	Commit     *Commit     `json:"commit,omitempty"`
	Repository *Repository `json:"repository,omitempty"`
}

// Participant is a pull request participant (reviewer/approver).
type Participant struct {
	User           User       `json:"user"`
	Role           string     `json:"role"`
	Approved       bool       `json:"approved"`
	State          string     `json:"state"`
	ParticipatedOn *time.Time `json:"participated_on,omitempty"`
}

// PullRequest is a Bitbucket pull request.
type PullRequest struct {
	ID                int           `json:"id"`
	Title             string        `json:"title"`
	Description       string        `json:"description"`
	State             string        `json:"state"`
	Draft             bool          `json:"draft"`
	Author            *User         `json:"author"`
	Source            PRRef         `json:"source"`
	Destination       PRRef         `json:"destination"`
	Reviewers         []User        `json:"reviewers"`
	Participants      []Participant `json:"participants"`
	CloseSourceBranch bool          `json:"close_source_branch"`
	CommentCount      int           `json:"comment_count"`
	TaskCount         int           `json:"task_count"`
	CreatedOn         time.Time     `json:"created_on"`
	UpdatedOn         time.Time     `json:"updated_on"`
	MergeCommit       *Commit       `json:"merge_commit"`
	ClosedBy          *User         `json:"closed_by"`
	Reason            string        `json:"reason"`
	Links             Links         `json:"links,omitzero"`
}

// MarshalJSON encodes pr with Reviewers and Participants as [] rather than
// null when empty or not reported.
func (pr PullRequest) MarshalJSON() ([]byte, error) {
	type plain PullRequest
	p := plain(pr)
	p.Reviewers = nonNil(p.Reviewers)
	p.Participants = nonNil(p.Participants)
	return marshalNoEscape(p)
}

// Content is rendered comment content.
type Content struct {
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
}

// CommentRef is a reference to a parent comment.
type CommentRef struct {
	ID int `json:"id"`
}

// Inline describes an inline (file/line) comment anchor.
type Inline struct {
	Path string `json:"path"`
	To   *int   `json:"to,omitempty"`
	From *int   `json:"from,omitempty"`
}

// Resolution is a comment resolution marker.
type Resolution struct {
	Type      string     `json:"type"`
	User      *User      `json:"user,omitempty"`
	CreatedOn *time.Time `json:"created_on,omitempty"`
}

// Comment is a pull request comment. Resolution, Parent and Inline are
// omitted from JSON when unset (unresolved, top-level, general comment).
type Comment struct {
	ID         int         `json:"id"`
	Content    Content     `json:"content"`
	User       User        `json:"user"`
	CreatedOn  time.Time   `json:"created_on"`
	UpdatedOn  time.Time   `json:"updated_on"`
	Deleted    bool        `json:"deleted"`
	Pending    bool        `json:"pending"`
	Resolution *Resolution `json:"resolution,omitempty"`
	Parent     *CommentRef `json:"parent,omitempty"`
	Inline     *Inline     `json:"inline,omitempty"`
	Links      Links       `json:"links,omitzero"`
}

// WorkspaceMember is an entry from the workspace members list.
type WorkspaceMember struct {
	User      User      `json:"user"`
	Workspace Workspace `json:"workspace"`
}

// MergeTaskStatus is the body of the async merge task-status poll.
type MergeTaskStatus struct {
	TaskStatus  string       `json:"task_status"`
	MergeResult *PullRequest `json:"merge_result"`
	TaskID      string       `json:"task_id"`
}

// nonNil returns s, or an empty non-nil slice when s is nil, so it encodes
// as [] instead of null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// marshalNoEscape encodes v like json.Marshal but without HTML escaping, so
// a MarshalJSON method does not turn "<", ">" and "&" into \u003c etc. when
// the caller's encoder has escaping turned off.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
