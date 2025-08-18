package slack

import (
	"context"
	"fmt"

	"github.com/ONSdigital/dis-web-mount-check/checker"
	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/ONSdigital/log.go/v2/log"
	"github.com/slack-go/slack"
)

// SlackNotifier implements checker.Notifier using NotifySlack().
type SlackNotifier struct{}

// Notify calls slack.NotifySlack.
func (SlackNotifier) Notify(ctx context.Context, cfg *config.Config, result string, state bool) {
	NotifySlack(ctx, cfg, result, state)
}

// Compile-time check that SlackNotifier satisfies the checker.Notifier interface.
var _ checker.Notifier = SlackNotifier{}

// NotifySlack sends message to Slack
func NotifySlack(ctx context.Context, cfg *config.Config, result string, state bool) {
	slackAPI := slack.New(cfg.SlackAPIToken)

	emoji := cfg.SlackAlarmEmoji
	if state {
		emoji = cfg.SlackOKEmoji
	}
	header := fmt.Sprintf("%s *'web-mount' app spread check state indicator:*", emoji)

	text := header + "\n" + result + "\n"

	channelID, timestamp, postErr := slackAPI.PostMessageContext(ctx, cfg.SlackAlarmChannel,
		slack.MsgOptionText(text, false),
		slack.MsgOptionUsername(cfg.SlackUserName),
	)

	if postErr != nil {
		log.Error(ctx, "failed to send Slack message", postErr)
	} else {
		log.Info(ctx, fmt.Sprintf("Slack message sent to channel %s at %s", channelID, timestamp))
	}
}
