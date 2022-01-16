package interaction

import (
	"context"
	"log"
	"os"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

type slashCommandHandler func(command slack.SlashCommand, slackWrapper *SlackWrapper) (*slack.Attachment, error)
type blockActionHandler func(interaction SlackInteraction) error

type SlackWrapper struct {
	token              string
	appToken           string
	client             *slack.Client
	commands           map[string]slashCommandHandler
	BlockActionHandler blockActionHandler
}

type SlackInteraction struct {
	ID          string
	ChannelID   string
	UserID      string
	ResponseUrl string
	Value       string
}

func NewSlackWrapper(token string, appToken string) *SlackWrapper {
	return &SlackWrapper{
		commands: make(map[string]slashCommandHandler),
		token:    token,
		appToken: appToken,
	}
}

func (c *SlackWrapper) AddCommand(command string, handler slashCommandHandler) {
	c.commands[command] = handler
}

func (c *SlackWrapper) Run() {

	c.client = slack.New(c.token, slack.OptionDebug(true), slack.OptionAppLevelToken(c.appToken))

	socketClient := socketmode.New(
		c.client,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	go func(ctx context.Context, client *slack.Client, socketClient *socketmode.Client) {
		for {
			select {
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socketClient.Events:
				switch event.Type {
				case socketmode.EventTypeSlashCommand:
					command, ok := event.Data.(slack.SlashCommand)
					if !ok {
						log.Printf("Could not type cast the message to a SlashCommand: %v\n", command)
						continue
					}
					respose, err := c.handleSlashCommand(command)
					if err != nil {
						log.Fatal(err)
					}

					socketClient.Ack(*event.Request, respose)

				case socketmode.EventTypeInteractive:
					interaction, ok := event.Data.(slack.InteractionCallback)
					if !ok {
						log.Printf("Could not type cast the message to a Interaction callback: %v\n", interaction)
						continue
					}

					err := c.handleInteractionEvent(interaction)
					if err != nil {
						log.Fatal(err)
					}

					socketClient.Ack(*event.Request)
				}
			}
		}
	}(ctx, c.client, socketClient)

	socketClient.Run()
}

func (c *SlackWrapper) handleInteractionEvent(interaction slack.InteractionCallback) error {
	switch interaction.Type {
	case slack.InteractionTypeBlockActions:
		for _, action := range interaction.ActionCallback.BlockActions {
			return c.BlockActionHandler(SlackInteraction{
				ID:          action.ActionID,
				ChannelID:   interaction.Container.ChannelID,
				UserID:      interaction.User.ID,
				Value:       action.Value,
				ResponseUrl: interaction.ResponseURL,
			})
		}
	default:
	}

	return nil
}

func (c *SlackWrapper) handleSlashCommand(command slack.SlashCommand) (*slack.Attachment, error) {
	if handle, ok := c.commands[command.Command]; ok {
		return handle(command, c)
	}
	return nil, nil
}

func (s *SlackWrapper) SendMessage(channelID, userID string, options ...slack.MsgOption) (string, error) {
	return s.client.PostEphemeral(channelID, userID, options...)
}
