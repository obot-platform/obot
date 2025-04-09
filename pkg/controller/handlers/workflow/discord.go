package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DiscordController struct {
	gptScript *gptscript.GPTScript
	sessions  map[string]*discordgo.Session
}

func NewDiscordController(gptScript *gptscript.GPTScript) *DiscordController {
	return &DiscordController{
		gptScript: gptScript,
		sessions:  make(map[string]*discordgo.Session),
	}
}

func (c *DiscordController) SubscribeToDiscord(req router.Request, _ router.Response) error {
	workflow := req.Object.(*v1.Workflow)
	if workflow.Spec.Manifest.OnDiscordMessage == nil {
		c.closeSession(workflow.Name)
		return nil
	}

	var thread v1.Thread
	if err := req.Get(&thread, workflow.Namespace, workflow.Spec.ThreadName); err != nil {
		return err
	}

	creds, err := c.gptScript.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{thread.Name},
	})
	if err != nil {
		return err
	}

	var discordToken string
	for _, cred := range creds {
		if cred.ToolName == "discord" {
			credValue, err := c.gptScript.RevealCredential(req.Ctx, []string{thread.Name}, cred.ToolName)
			if err != nil {
				return err
			}
			discordToken = credValue.Env["DISCORD_TOKEN"]
			break
		}
	}

	if discordToken == "" {
		return fmt.Errorf("discord bot token not found for workflow %s", workflow.Name)
	}

	if session, exists := c.sessions[workflow.Name]; exists {
		if session.Token != discordToken {
			c.closeSession(workflow.Name)
		} else {
			return nil
		}
	}

	session, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		return err
	}
	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	session.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		isMentioned := false
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				isMentioned = true
				break
			}
		}

		if !isMentioned {
			return
		}

		c.triggerWorkflow(req, workflow, thread, m)
	})

	if err := session.Open(); err != nil {
		return err
	}

	c.sessions[workflow.Name] = session
	return nil
}

func (c *DiscordController) closeSession(workflowName string) {
	if session, exists := c.sessions[workflowName]; exists {
		session.Close()
		delete(c.sessions, workflowName)
	}
}

func (c *DiscordController) triggerWorkflow(req router.Request, workflow *v1.Workflow, thread v1.Thread, m *discordgo.MessageCreate) {
	payload := map[string]interface{}{
		"type": "discord",
		"event": map[string]interface{}{
			"content":    m.Content,
			"channelID":  m.ChannelID,
			"messageID":  m.ID,
			"authorID":   m.Author.ID,
			"authorName": m.Author.Username,
			"guildID":    m.GuildID,
			"threadID":   m.ChannelID,
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling Discord payload: %v\n", err)
		return
	}

	err = req.Client.Create(req.Ctx, &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Namespace:    thread.Namespace,
		},
		Spec: v1.WorkflowExecutionSpec{
			Input:        string(payloadBytes),
			ThreadName:   thread.Name,
			WorkflowName: workflow.Name,
		},
	})
	if err != nil {
		fmt.Printf("Error creating workflow execution for Discord message: %v\n", err)
	}
}
