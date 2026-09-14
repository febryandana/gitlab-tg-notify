package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/febryandana/gitlab-tg-notify/internal/model"
)

// retryDelays implements the fixed 3-try backoff from PRD §13.4.
var retryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// Client is a thin wrapper around gotgbot that knows how to send a
// NotificationEvent as a new message or as a reply to an existing one.
type Client struct {
	bot       *gotgbot.Bot
	chatID    int64
	useTopics bool
	log       *slog.Logger
}

// NewClient creates a gotgbot.Bot from the given token and wraps it.
func NewClient(token string, chatID int64, useTopics bool, log *slog.Logger) (*Client, error) {
	bot, err := gotgbot.NewBot(token, nil)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	return &Client{bot: bot, chatID: chatID, useTopics: useTopics, log: log}, nil
}

// SendOpts controls how one message is sent.
type SendOpts struct {
	// ThreadID is the forum topic to post into. Ignored unless useTopics.
	ThreadID *int64
	// ReplyToMessageID, if non-zero, makes this message a reply.
	ReplyToMessageID int64
}

// Send delivers text for event e, retrying on failure per PRD §13.4
// (3 attempts, 2s/5s/10s backoff). Returns the sent message's ID so callers
// can record it as a new root message.
func (c *Client) Send(ctx context.Context, text string, opts SendOpts) (int64, error) {
	sendOpts := &gotgbot.SendMessageOpts{
		ParseMode: gotgbot.ParseModeHTML,
	}

	if c.useTopics && opts.ThreadID != nil {
		sendOpts.MessageThreadId = *opts.ThreadID
	}

	if opts.ReplyToMessageID != 0 {
		sendOpts.ReplyParameters = &gotgbot.ReplyParameters{
			MessageId:                opts.ReplyToMessageID,
			AllowSendingWithoutReply: true,
		}
	}

	var lastErr error
	for attempt := 0; ; attempt++ {
		msg, err := c.bot.SendMessageWithContext(ctx, c.chatID, text, sendOpts)
		if err == nil {
			return msg.MessageId, nil
		}
		lastErr = err

		if attempt >= len(retryDelays) {
			break
		}

		if c.log != nil {
			c.log.Warn("telegram send failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
		}

		select {
		case <-time.After(retryDelays[attempt]):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	return 0, fmt.Errorf("send telegram message after %d attempts: %w", len(retryDelays)+1, lastErr)
}

// SendEvent formats and sends a NotificationEvent.
func (c *Client) SendEvent(ctx context.Context, e model.NotificationEvent, threadID *int64, replyToMessageID int64) (int64, error) {
	text := FormatMessage(e)
	return c.Send(ctx, text, SendOpts{ThreadID: threadID, ReplyToMessageID: replyToMessageID})
}
