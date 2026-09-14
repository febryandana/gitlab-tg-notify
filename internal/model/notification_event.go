// Package model defines the internal, transport-agnostic types shared
// between the webhook receiver and the Telegram sender.
package model

// EventKind identifies what happened to a GitLab issue.
type EventKind string

const (
	EventOpened    EventKind = "opened"
	EventUpdated   EventKind = "updated"
	EventCommented EventKind = "commented"
	EventClosed    EventKind = "closed"
	EventReopened  EventKind = "reopened"
)

// NotificationEvent is the internal, event-type-agnostic shape that both
// IssueEvent and IssueCommentEvent webhook payloads get converted into.
// The telegram and store packages only need to know about this type, not
// about GitLab's webhook JSON shapes.
type NotificationEvent struct {
	ProjectID      int
	ProjectPath    string
	IssueIID       int
	Kind           EventKind
	Title          string
	URL            string
	Description    string // only set for EventOpened
	CommentBody    string // only set for EventCommented
	ActorName      string
	IsConfidential bool
	// TimeSpentSecs carries GitLab's cumulative "total time spent" on the
	// issue as parsed off the webhook. The webhook handler overwrites this
	// with the delta attributable to this specific comment (0 if none)
	// before handing the event to the formatter — see
	// webhook.Handler.trackTimeSpent.
	TimeSpentSecs int64
}
