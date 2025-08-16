package slack

import (
	"context"
	"fmt"

	"github.com/ONSdigital/dis-web-mount-check/config"
	"github.com/ONSdigital/log.go/v2/log"
	"github.com/slack-go/slack"
)

// NotifySlack sends message to Slack
func NotifySlack(ctx context.Context, cfg *config.Config, result string, state bool) {
	slackAPI := slack.New(cfg.SlackAPIToken)

	emoji := cfg.SlackAlarmEmoji
	if state {
		emoji = cfg.SlackOKEmoji
	}
	header := fmt.Sprintf("%s *'web-mount' app spread check (test - ignore):*", emoji)

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
