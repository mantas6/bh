package api

import "time"

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
	HTML  Link        `json:"html"`
	Self  Link        `json:"self"`
	Clone []CloneLink `json:"clone"`
}

// User is a Bitbucket account.
type User struct {
	UUID        string `json:"uuid"`
	AccountID   string `json:"account_id"`
	Nickname    string `json:"nickname"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Links       Links  `json:"links"`
}

// Workspace is a Bitbucket workspace.
type Workspace struct {
	UUID string `json:"uuid"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// Branch is a repository branch / merge configuration.
type Branch struct {
	Name                 string   `json:"name"`
	DefaultMergeStrategy string   `json:"default_merge_strategy"`
	MergeStrategies      []string `json:"merge_strategies"`
}

// Repository is a Bitbucket repository.
type Repository struct {
	UUID       string      `json:"uuid"`
	Name       string      `json:"name"`
	FullName   string      `json:"full_name"`
	Slug       string      `json:"slug"`
	IsPrivate  bool        `json:"is_private"`
	MainBranch *Branch     `json:"mainbranch"`
	Links      Links       `json:"links"`
	Owner      *User       `json:"owner"`
	Workspace  *Workspace  `json:"workspace"`
	Parent     *Repository `json:"parent"`
}

// Commit is a minimal commit reference.
type Commit struct {
	Hash string `json:"hash"`
}

// PRRef is a pull request source/destination endpoint.
type PRRef struct {
	Branch     Branch      `json:"branch"`
	Commit     *Commit     `json:"commit"`
	Repository *Repository `json:"repository"`
}

// Participant is a pull request participant (reviewer/approver).
type Participant struct {
	User           User       `json:"user"`
	Role           string     `json:"role"`
	Approved       bool       `json:"approved"`
	State          string     `json:"state"`
	ParticipatedOn *time.Time `json:"participated_on"`
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
	Links             Links         `json:"links"`
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
	To   *int   `json:"to"`
	From *int   `json:"from"`
}

// Resolution is a comment resolution marker.
type Resolution struct {
	Type      string     `json:"type"`
	User      *User      `json:"user"`
	CreatedOn *time.Time `json:"created_on"`
}

// Comment is a pull request comment.
type Comment struct {
	ID         int         `json:"id"`
	Content    Content     `json:"content"`
	User       User        `json:"user"`
	CreatedOn  time.Time   `json:"created_on"`
	UpdatedOn  time.Time   `json:"updated_on"`
	Deleted    bool        `json:"deleted"`
	Pending    bool        `json:"pending"`
	Resolution *Resolution `json:"resolution"`
	Parent     *CommentRef `json:"parent"`
	Inline     *Inline     `json:"inline"`
	Links      Links       `json:"links"`
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
