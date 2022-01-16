package main

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/sergiupantiru/voteish/internal/interaction"
	"github.com/sergiupantiru/voteish/internal/repository"
	"github.com/sergiupantiru/voteish/internal/voting"

	"github.com/slack-go/slack"
	"github.com/spf13/viper"
)

var repo *repository.Repository = repository.NewRepository()

func main() {
	log.Default().SetPrefix("votingApp:")
	log.Default().SetFlags(log.Lshortfile | log.LstdFlags)

	viper.SetEnvPrefix("voteish")
	viper.BindEnv("token")
	viper.BindEnv("appToken")

	token := viper.GetString("TOKEN")
	appToken := viper.GetString("APPTOKEN")

	var slackWrapper *interaction.SlackWrapper = interaction.NewSlackWrapper(token, appToken)

	slackWrapper.AddCommand("/voteish", handleVoteCommand)
	slackWrapper.BlockActionHandler = handleUserInteraction

	slackWrapper.Run()
}

func handleUserInteraction(interaction interaction.SlackInteraction) error {
	splits := strings.Split(interaction.Value, `|`)
	sessionId := splits[0]
	actionValue := splits[1]
	if session, ok := repo.Get(sessionId); ok {
		log.Printf("User %s performed action %s on session %s with values %s", interaction.ID, interaction.UserID, sessionId, splits[1])
		switch interaction.ID {
		case "participate":
			return session.UserParticipateSession(interaction.UserID, interaction.ResponseUrl)
		case "skip":
			return session.UserSkipSession(interaction.ChannelID, interaction.UserID, interaction.ResponseUrl)
		case "start":
			return session.StartSession()
		default:
			if strings.HasPrefix(interaction.ID, "vote") {
				return session.UserVoted(interaction.UserID, actionValue)
			}
			return nil
		}
	}
	return nil
}

func handleVoteCommand(command slack.SlashCommand, slackWrapper *interaction.SlackWrapper) (*slack.Attachment, error) {
	votingSession := voting.NewVotingSession(slackWrapper, command)
	if votingSession == nil {
		return nil, nil
	}
	repo.AddSession(votingSession)
	log.Printf("Voting session started %s", toJson(*votingSession))

	votingSession.SendInviteToAllUser()

	return nil, nil
}

func toJson(obj interface{}) string {
	val, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(val)
}
