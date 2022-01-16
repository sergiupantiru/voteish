package voting

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sergiupantiru/voteish/internal/interaction"
	"github.com/slack-go/slack"
)

type Voter struct {
	Mention       string `json:"mention"`
	Vote          string `json:"vote"`
	ResponseUrl   string `json:"responseUrl"`
	Participating int    `json:"participating"`
}

const (
	NA            = 0
	Participating = 1
	Skip          = 2
)

type VotingSession struct {
	Voters       map[string]*Voter `json:"voters"`
	SessionId    string            `json:"sessionId"`
	Channel      string            `json:"channel"`
	Message      string            `json:"message"`
	Owner        string            `json:"owner"`
	OwnerId      string            `json:"ownerId"`
	Started      bool              `json:"started"`
	slackWrapper *interaction.SlackWrapper
}

func NewVotingSession(slackWrapper *interaction.SlackWrapper, command slack.SlashCommand) *VotingSession {
	var userIdRegex *regexp.Regexp = regexp.MustCompile(`\<\@([A-Z-0-9]+)\|*([a-zA-Z0-9\-_\._]*)\>`)
	match := userIdRegex.FindAllStringSubmatch(command.Text, -1)
	if len(match) == 0 {
		return nil
	}
	firstUserIndex := strings.Index(command.Text, match[0][0])
	votingSessionMessage := command.Text[:firstUserIndex]

	voters := make(map[string]*Voter)
	for i := 0; i < len(match); i++ {
		voters[match[i][1]] = &Voter{
			Mention:       match[i][0],
			Participating: NA,
		}

	}

	sessionId := strconv.Itoa(int(time.Now().UnixMilli()))
	return &VotingSession{
		Voters:       voters,
		SessionId:    sessionId,
		Channel:      command.ChannelID,
		Message:      votingSessionMessage,
		Owner:        command.UserName,
		OwnerId:      command.UserID,
		Started:      false,
		slackWrapper: slackWrapper,
	}
}

func (session *VotingSession) participatingVoters() map[string]*Voter {
	pVoters := make(map[string]*Voter)
	for key, val := range session.Voters {
		if val.Participating != Skip {
			pVoters[key] = val
		}
	}
	return pVoters
}

func (session *VotingSession) areAllVotersSkipping() bool {
	for _, val := range session.Voters {
		if val.Participating != Skip {
			return false
		}
	}
	return true
}

func (session *VotingSession) StartSession() error {
	session.Started = true
	session.markInactiveUsersAsSkipping()
	session.SendMessageToAllUsers()
	return nil
}

func (session *VotingSession) markInactiveUsersAsSkipping() {
	for _, user := range session.Voters {
		if user.Participating == NA {
			user.Participating = Skip
		}
	}
}

func (session *VotingSession) UserSkipSession(channelId string, userId string, responseUrl string) error {
	session.slackWrapper.SendMessage(channelId, userId, slack.MsgOptionDeleteOriginal(responseUrl))
	if voter, ok := session.Voters[userId]; ok {
		voter.Participating = Skip
	}
	return nil
}

func (session *VotingSession) UserParticipateSession(userId string, responseUrl string) error {
	if voter, ok := session.Voters[userId]; ok {
		voter.Participating = Participating
		voter.ResponseUrl = responseUrl
	}
	session.SendMessageToAllUsers()
	return nil
}

func (session *VotingSession) UserVoted(userId string, vote string) error {
	if voter, ok := session.Voters[userId]; ok {
		voter.Vote = vote
	}
	session.SendMessageToAllUsers()
	return nil
}

func (session *VotingSession) SendInviteToAllUser() {
	blocks := session.createInviteMessageBlocks()

	for userId := range session.Voters {
		session.slackWrapper.SendMessage(session.Channel, userId, slack.MsgOptionBlocks(blocks...))
	}
}

func (session *VotingSession) SendMessageToAllUsers() {

	votes := make([]int, 0)
	for _, user := range session.participatingVoters() {
		if user.Vote == "" {
			continue
		}
		vote, _ := strconv.Atoi(user.Vote)
		votes = append(votes, vote)
	}

	voteClosed := len(votes) == len(session.participatingVoters())

	blocks := session.createMessageBlocks(&votes, voteClosed)

	for userId, currentUser := range session.participatingVoters() {
		if userId == session.OwnerId && !session.Started {
			session.slackWrapper.SendMessage(session.Channel, userId, slack.MsgOptionBlocks((append(blocks, session.createStartVoteAction()))...), slack.MsgOptionReplaceOriginal(currentUser.ResponseUrl))
		} else {
			session.slackWrapper.SendMessage(session.Channel, userId, slack.MsgOptionBlocks(blocks...), slack.MsgOptionReplaceOriginal(currentUser.ResponseUrl))
		}
	}
}

func (session *VotingSession) createInviteMessageBlocks() []slack.Block {
	blocks := []slack.Block{
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: fmt.Sprintf("%s invited you to vote.", session.Owner),
			},
			nil,
			nil,
		),
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: session.Message,
			},
			nil,
			nil,
		),
	}
	blocks = append(blocks, slack.NewActionBlock(
		"invite",
		*slack.NewButtonBlockElement(
			"participate",
			fmt.Sprintf("%s|participate", session.SessionId),
			&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Participate",
			},
		),
		*slack.NewButtonBlockElement(
			"skip",
			fmt.Sprintf("%s|skip", session.SessionId),
			&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Skip",
			},
		)))
	return blocks
}

func (session *VotingSession) createMessageBlocks(votes *[]int, voteClosed bool) []slack.Block {
	votersBlocks := make([]*slack.TextBlockObject, 0)

	for _, voter := range session.participatingVoters() {
		if voteClosed {
			votersBlocks = append(votersBlocks, &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "*" + voter.Vote + "*    " + voter.Mention,
			})
		} else if voter.Participating == NA {
			votersBlocks = append(votersBlocks, &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: ":grey_question: " + voter.Mention,
			})
		} else if voter.Vote == "" {
			votersBlocks = append(votersBlocks, &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: ":ballot_box_with_check: " + voter.Mention,
			})
		} else {
			votersBlocks = append(votersBlocks, &slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: ":white_check_mark: " + voter.Mention,
			})
		}
	}
	blocks := []slack.Block{
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: fmt.Sprintf("%s invited you to vote.", session.Owner),
			},
			nil,
			nil,
		),
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: session.Message,
			},
			nil,
			nil,
		),
		slack.NewDividerBlock(),
		slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: "*Voters*: ",
			},
			votersBlocks,
			nil,
		),
		slack.NewDividerBlock(),
	}

	if !session.Started {
		return blocks
	}

	if !voteClosed {
		blocks = append(blocks, slack.NewActionBlock(
			"vote",
			session.createUserActions()...,
		))
	} else {
		blocks = append(blocks, slack.NewSectionBlock(
			&slack.TextBlockObject{
				Type: slack.MarkdownType,
				Text: fmt.Sprintf("Average : %.1f", getAverage(votes)),
			},
			nil,
			nil,
		))
		for key, val := range countOccurrences(votes) {
			blocks = append(blocks, slack.NewSectionBlock(
				&slack.TextBlockObject{
					Type: slack.MarkdownType,
					Text: fmt.Sprintf("*%d*: %s", key, getProgress(val)),
				},
				nil,
				nil,
			))
		}
	}
	return blocks
}

func (session *VotingSession) createStartVoteAction() slack.Block {
	return slack.NewActionBlock(
		"vote",
		*slack.NewButtonBlockElement(
			"start",
			fmt.Sprintf("%s|%s", session.SessionId, "start"),
			&slack.TextBlockObject{
				Type: slack.PlainTextType,
				Text: "Start",
			},
		),
	)
}

func (session *VotingSession) createUserActions() []slack.BlockElement {
	actions := make([]slack.BlockElement, 0)
	for _, action := range []string{"1", "2", "3", "5", "8", "13", "21", "100"} {
		actions = append(actions,
			*slack.NewButtonBlockElement(
				"vote"+action,
				fmt.Sprintf("%s|%s", session.SessionId, action),
				&slack.TextBlockObject{
					Type: slack.PlainTextType,
					Text: action,
				},
			))
	}
	return actions
}

func getProgress(count int) string {
	val := ""
	for i := 0; i < count; i++ {
		val += ":large_blue_square:"
	}
	return val
}

func countOccurrences(votes *[]int) map[int]int {
	occurrences := make(map[int]int)
	for _, num := range *votes {
		occurrences[num] = occurrences[num] + 1
	}
	return occurrences
}

func getAverage(votes *[]int) float64 {
	sum := 0
	for _, vote := range *votes {
		sum += vote
	}
	return float64(sum) / float64(len(*votes))
}
