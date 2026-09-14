// Package webhook implements the GitLab webhook receiver: auth, payload
// parsing, allow-list filtering, and handing off to the Telegram sender
// (PRD §7).
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/febryandana/gitlab-tg-notify/internal/config"
	"github.com/febryandana/gitlab-tg-notify/internal/model"
	"github.com/febryandana/gitlab-tg-notify/internal/store"
)

// Sender is the subset of telegram.Client the handler depends on. Declaring
// it here (rather than importing the concrete type) lets tests supply a
// fake without pulling in gotgbot.
type Sender interface {
	SendEvent(ctx context.Context, e model.NotificationEvent, threadID *int64, replyToMessageID int64) (int64, error)
}

// ThreadStore is the subset of store.IssueThreadStore the handler depends on.
type ThreadStore interface {
	Insert(ctx context.Context, projectID, issueIID int, rootMessageID int64, isConfidential bool) error
	GetRootMessageID(ctx context.Context, projectID, issueIID int) (int64, error)
}

// TimeTracking is the subset of store.TimeTrackingStore the handler depends
// on, used to turn GitLab's cumulative "total time spent" into the amount
// logged by one specific comment.
type TimeTracking interface {
	GetLastTotalSecs(ctx context.Context, projectID, issueIID int) (int64, error)
	SetLastTotalSecs(ctx context.Context, projectID, issueIID int, secs int64) error
}

// Handler is the http.Handler for the GitLab webhook endpoint.
type Handler struct {
	cfg          *config.Config
	sender       Sender
	store        ThreadStore
	timeTracking TimeTracking
	log          *slog.Logger
}

func NewHandler(cfg *config.Config, sender Sender, threadStore ThreadStore, timeTracking TimeTracking, log *slog.Logger) *Handler {
	return &Handler{cfg: cfg, sender: sender, store: threadStore, timeTracking: timeTracking, log: log}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !verifyToken(r, h.cfg.GitLab.WebhookSecret) {
		h.log.Warn("webhook token mismatch", "remote_addr", r.RemoteAddr)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("read webhook body failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	eventTypeHeader := gitlab.EventType(r.Header.Get("X-Gitlab-Event"))

	event, err := parseEvent(eventTypeHeader, body)
	if err != nil {
		// Unsupported event type, action, or noteable kind — expected,
		// everyday filtering (PRD §13.1 style): debug only.
		h.log.Debug("ignoring webhook event", "event_type", string(eventTypeHeader), "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	project, ok := h.cfg.FindProject(event.ProjectID)
	if !ok {
		h.log.Debug("event dropped: project not on allow-list", "gitlab_project_id", event.ProjectID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Hand off and return 200 immediately (PRD §7 step 6-7); the actual
	// Telegram send + store write happen off the request goroutine.
	go h.process(context.Background(), *event, project)

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) process(ctx context.Context, e model.NotificationEvent, project config.ProjectConfig) {
	// PRD §13.2: this should never happen if config.Validate ran at
	// startup, but a config hot-reload could add a project without a
	// thread ID while use_topics is still true.
	if h.cfg.Telegram.UseTopics && project.ThreadID == nil {
		h.log.Error("event dropped: no topic mapped for project",
			"gitlab_project_id", e.ProjectID,
			"gitlab_project_path", e.ProjectPath,
			"gitlab_issue_iid", e.IssueIID,
			"event_kind", string(e.Kind),
		)
		return
	}

	e.TimeSpentSecs = h.trackTimeSpent(ctx, e)

	if e.Kind == model.EventOpened {
		h.sendOpened(ctx, e, project)
		return
	}

	h.sendFollowUp(ctx, e, project)
}

// trackTimeSpent updates the running-total baseline for this issue (so the
// next comment's delta is computed correctly, even if time was logged via a
// non-comment path) and returns the amount attributable to this event: for
// a comment, current total minus the last known total (never negative,
// e.g. if time was removed); for every other event kind, 0, since time
// spent is only ever displayed per-comment now.
func (h *Handler) trackTimeSpent(ctx context.Context, e model.NotificationEvent) int64 {
	currentTotal := e.TimeSpentSecs

	last, err := h.timeTracking.GetLastTotalSecs(ctx, e.ProjectID, e.IssueIID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		last = 0
	case err != nil:
		h.log.Error("failed to look up time-tracking baseline",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"error", err,
		)
		last = 0
	}

	if err := h.timeTracking.SetLastTotalSecs(ctx, e.ProjectID, e.IssueIID, currentTotal); err != nil {
		h.log.Error("failed to save time-tracking baseline",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"error", err,
		)
	}

	if e.Kind != model.EventCommented {
		return 0
	}

	delta := currentTotal - last
	if delta < 0 {
		delta = 0
	}
	return delta
}

func (h *Handler) sendOpened(ctx context.Context, e model.NotificationEvent, project config.ProjectConfig) {
	msgID, err := h.sender.SendEvent(ctx, e, project.ThreadID, 0)
	if err != nil {
		h.log.Error("telegram send failed",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"event_kind", string(e.Kind),
			"error", err,
		)
		return
	}

	if err := h.store.Insert(ctx, e.ProjectID, e.IssueIID, msgID, e.IsConfidential); err != nil {
		h.log.Error("failed to save root message id",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"error", err,
		)
	}
}

func (h *Handler) sendFollowUp(ctx context.Context, e model.NotificationEvent, project config.ProjectConfig) {
	var replyToMessageID int64

	rootID, err := h.store.GetRootMessageID(ctx, e.ProjectID, e.IssueIID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// PRD §13.3: send standalone rather than failing the request.
		h.log.Warn("no root message found, sending as standalone message",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"event_kind", string(e.Kind),
		)
	case err != nil:
		h.log.Error("failed to look up root message id",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"error", err,
		)
		return
	default:
		replyToMessageID = rootID
	}

	if _, err := h.sender.SendEvent(ctx, e, project.ThreadID, replyToMessageID); err != nil {
		h.log.Error("telegram send failed",
			"gitlab_project_id", e.ProjectID,
			"gitlab_issue_iid", e.IssueIID,
			"event_kind", string(e.Kind),
			"error", err,
		)
	}
}

// parseEvent converts a raw GitLab webhook payload into a NotificationEvent,
// per the mapping in PRD §11. It returns an error for anything out of
// scope: an unrecognized event type, an unrecognized issue action, or a
// note on something other than an issue (MR/commit/snippet comments).
func parseEvent(eventType gitlab.EventType, body []byte) (*model.NotificationEvent, error) {
	switch eventType {
	case gitlab.EventTypeIssue, gitlab.EventConfidentialIssue:
		return parseIssueEvent(body)
	case gitlab.EventTypeNote, gitlab.EventConfidentialNote:
		return parseNoteEvent(body)
	default:
		return nil, fmt.Errorf("unsupported event type %q", eventType)
	}
}

func parseIssueEvent(body []byte) (*model.NotificationEvent, error) {
	var e gitlab.IssueEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("unmarshal issue event: %w", err)
	}

	kind, ok := issueActionToKind(e.ObjectAttributes.Action)
	if !ok {
		return nil, fmt.Errorf("unsupported issue action %q", e.ObjectAttributes.Action)
	}

	ne := &model.NotificationEvent{
		ProjectID:      int(e.Project.ID),
		ProjectPath:    e.Project.PathWithNamespace,
		IssueIID:       int(e.ObjectAttributes.IID),
		Kind:           kind,
		Title:          e.ObjectAttributes.Title,
		URL:            e.ObjectAttributes.URL,
		IsConfidential: e.ObjectAttributes.Confidential,
		TimeSpentSecs:  e.ObjectAttributes.TotalTimeSpent,
	}
	if e.User != nil {
		ne.ActorName = e.User.Name
	}
	if kind == model.EventOpened {
		ne.Description = e.ObjectAttributes.Description
	}

	return ne, nil
}

func parseNoteEvent(body []byte) (*model.NotificationEvent, error) {
	var e gitlab.IssueCommentEvent
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("unmarshal note event: %w", err)
	}

	if e.ObjectAttributes.NoteableType != "Issue" {
		// Comment on an MR/commit/snippet — out of scope for v1 (PRD §3, §6).
		return nil, fmt.Errorf("note is not on an issue (noteable_type=%q)", e.ObjectAttributes.NoteableType)
	}

	ne := &model.NotificationEvent{
		ProjectID:      int(e.ProjectID),
		ProjectPath:    e.Project.PathWithNamespace,
		IssueIID:       int(e.Issue.IID),
		Kind:           model.EventCommented,
		Title:          e.Issue.Title,
		URL:            e.ObjectAttributes.URL,
		CommentBody:    e.ObjectAttributes.Note,
		IsConfidential: e.Issue.Confidential,
		TimeSpentSecs:  e.Issue.TotalTimeSpent,
	}
	if e.User != nil {
		ne.ActorName = e.User.Name
	}

	return ne, nil
}

func issueActionToKind(action string) (model.EventKind, bool) {
	switch action {
	case "open":
		return model.EventOpened, true
	case "update":
		return model.EventUpdated, true
	case "close":
		return model.EventClosed, true
	case "reopen":
		return model.EventReopened, true
	default:
		return "", false
	}
}
