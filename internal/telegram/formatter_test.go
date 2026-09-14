package telegram

import (
	"strings"
	"testing"

	"github.com/febryandana/gitlab-tg-notify/internal/model"
)

func TestFormatMessage_EscapesHTML(t *testing.T) {
	e := model.NotificationEvent{
		Kind:      model.EventOpened,
		IssueIID:  12,
		Title:     "<script>alert(1)</script>",
		URL:       "https://gitlab.example.com/issues/12",
		ActorName: "A & B",
	}

	got := FormatMessage(e)

	if strings.Contains(got, "<script>") {
		t.Errorf("FormatMessage did not escape title, got: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("FormatMessage missing escaped title, got: %s", got)
	}
	if !strings.Contains(got, "A &amp; B") {
		t.Errorf("FormatMessage did not escape actor name, got: %s", got)
	}
}

func TestFormatMessage_TruncatesLongDescription(t *testing.T) {
	e := model.NotificationEvent{
		Kind:        model.EventOpened,
		IssueIID:    1,
		Title:       "t",
		URL:         "https://gitlab.example.com/issues/1",
		ActorName:   "actor",
		Description: strings.Repeat("x", 10000),
	}

	got := FormatMessage(e)

	if len(got) > maxMessageLen {
		t.Fatalf("FormatMessage len = %d, want <= %d", len(got), maxMessageLen)
	}
	if !strings.Contains(got, readMoreSuffix) {
		t.Errorf("FormatMessage missing truncation suffix, got tail: %q", got[len(got)-40:])
	}
}

func TestFormatMessage_ShortMessageUnaffected(t *testing.T) {
	e := model.NotificationEvent{
		Kind:      model.EventClosed,
		IssueIID:  7,
		Title:     "short title",
		URL:       "https://gitlab.example.com/issues/7",
		ActorName: "actor",
	}

	got := FormatMessage(e)

	if strings.Contains(got, readMoreSuffix) {
		t.Errorf("short message should not be truncated, got: %s", got)
	}
	if !strings.Contains(got, "closed") {
		t.Errorf("expected closed-event wording, got: %s", got)
	}
}

func TestFormatMessage_TimeSpent(t *testing.T) {
	commentWithTime := model.NotificationEvent{
		Kind:          model.EventCommented,
		IssueIID:      3,
		Title:         "t",
		URL:           "https://gitlab.example.com/issues/3",
		ActorName:     "actor",
		TimeSpentSecs: 5400, // this comment's delta, e.g. "/spend 1h30m"
	}
	got := FormatMessage(commentWithTime)
	if !strings.Contains(got, "⏱ Time spent: 1h 30m") {
		t.Errorf("expected time-spent line, got: %s", got)
	}

	commentNoTime := model.NotificationEvent{
		Kind:      model.EventCommented,
		IssueIID:  4,
		Title:     "t",
		URL:       "https://gitlab.example.com/issues/4",
		ActorName: "actor",
	}
	got = FormatMessage(commentNoTime)
	if !strings.Contains(got, "⏱ Time spent: 0m") {
		t.Errorf("expected time-spent line showing 0m for a comment with no logged time, got: %s", got)
	}

	nonCommentEvent := model.NotificationEvent{
		Kind:          model.EventUpdated,
		IssueIID:      5,
		Title:         "t",
		URL:           "https://gitlab.example.com/issues/5",
		ActorName:     "actor",
		TimeSpentSecs: 3600,
	}
	got = FormatMessage(nonCommentEvent)
	if strings.Contains(got, "Time spent") {
		t.Errorf("expected no time-spent line for a non-comment event, got: %s", got)
	}
}
