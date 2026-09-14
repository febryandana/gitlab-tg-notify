// Package telegram builds Telegram message text from a NotificationEvent
// and sends it via gotgbot.
package telegram

import (
	"fmt"
	"strings"

	"github.com/febryandana/gitlab-tg-notify/internal/model"
)

// maxMessageLen is Telegram's hard cap on message text (PRD §12).
const maxMessageLen = 4096

const readMoreSuffix = "… (read more on GitLab)"

// emoji is the small, easy-to-edit set of prefixes used per event kind
// (PRD §12's "rules" note). Change or clear these to turn emoji off.
var emoji = map[model.EventKind]string{
	model.EventOpened:    "🆕",
	model.EventUpdated:   "✏️",
	model.EventCommented: "💬",
	model.EventClosed:    "✅",
	model.EventReopened:  "🔁",
}

// escapeHTML escapes only the characters Telegram's HTML parse_mode treats
// specially (PRD §12): '&' must be replaced first so it doesn't re-escape
// the entities just produced for '<' and '>'.
func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	)
	return r.Replace(s)
}

// FormatMessage renders the Telegram HTML text for one NotificationEvent,
// using the templates in PRD §12.
func FormatMessage(e model.NotificationEvent) string {
	title := escapeHTML(e.Title)
	url := e.URL // URLs are not escaped: they're placed outside any tag attribute.

	header := formatHeader(e, title)
	if timeLine := formatTimeSpent(e); timeLine != "" {
		header += "\n" + timeLine
	}
	body := formatBody(e)

	return assembleMessage(header, url, body)
}

// formatTimeSpent returns the "⏱ Time spent" line for the time logged by
// this specific comment (not the issue-wide total), or "" for event kinds
// that aren't a comment at all. e.TimeSpentSecs is 0 when the comment didn't
// log any time, which renders as "0m".
func formatTimeSpent(e model.NotificationEvent) string {
	if e.Kind != model.EventCommented {
		return ""
	}
	return fmt.Sprintf("⏱ Time spent: %s", formatDuration(e.TimeSpentSecs))
}

// formatDuration renders seconds as "1h 30m" / "1h" / "0m".
func formatDuration(secs int64) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func formatHeader(e model.NotificationEvent, title string) string {
	icon := emoji[e.Kind]

	switch e.Kind {
	case model.EventOpened:
		return fmt.Sprintf("%s <b>%s</b>\n📌 <b>Issue ID :</b> #%d\n👤 Opened by: %s",
			icon, title, e.IssueIID, escapeHTML(e.ActorName))
	case model.EventUpdated:
		return fmt.Sprintf("%s <b>%s</b> — updated\n📌 <b>Issue ID :</b> #%d\n👤 Updated by: %s",
			icon, title, e.IssueIID, escapeHTML(e.ActorName))
	case model.EventCommented:
		return fmt.Sprintf("%s <b>%s</b>\n📌 <b>Issue ID :</b> #%d\n👤 Commented by: %s",
			icon, title, e.IssueIID, escapeHTML(e.ActorName))
	case model.EventClosed:
		return fmt.Sprintf("%s <b>%s</b> — closed\n📌 <b>Issue ID :</b> #%d\n👤 Closed by: %s",
			icon, title, e.IssueIID, escapeHTML(e.ActorName))
	case model.EventReopened:
		return fmt.Sprintf("%s <b>%s</b> — reopened\n📌 <b>Issue ID :</b> #%d\n👤 Reopened by: %s",
			icon, title, e.IssueIID, escapeHTML(e.ActorName))
	default:
		return fmt.Sprintf("<b>%s</b>\n📌 <b>Issue ID :</b> #%d\n👤 %s",
			title, e.IssueIID, escapeHTML(e.ActorName))
	}
}

// formatBody returns the escaped, trailing free-text block for the event
// (description for "opened", comment text for "commented"), or "" for
// event kinds that carry no such text.
func formatBody(e model.NotificationEvent) string {
	switch e.Kind {
	case model.EventOpened:
		return escapeHTML(e.Description)
	case model.EventCommented:
		return escapeHTML(e.CommentBody)
	default:
		return ""
	}
}

// assembleMessage joins header + link + optional body, truncating body (not
// header or link) so the total stays within Telegram's 4096-char limit, per
// PRD §12.
func assembleMessage(header, url, body string) string {
	fixed := header + "\n🔗 " + url
	if body == "" {
		return fixed
	}

	full := fixed + "\n\n" + body
	if len(full) <= maxMessageLen {
		return full
	}

	// Budget for the body: total limit minus everything else (fixed part,
	// the two newlines joining it to body, and the suffix we'll append).
	budget := maxMessageLen - len(fixed) - len("\n\n") - len(readMoreSuffix)
	if budget < 0 {
		budget = 0
	}

	truncated := truncateRunes(body, budget) + readMoreSuffix
	return fixed + "\n\n" + truncated
}

// truncateRunes cuts s to at most n bytes without splitting a UTF-8
// rune in half.
func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Walk backwards from n until we're not in the middle of a rune.
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool {
	return b&0xC0 != 0x80
}
