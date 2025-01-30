package cleanup

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
)

type Credentials struct {
	gClient *gptscript.GPTScript
}

func NewCredentials(gClient *gptscript.GPTScript) *Credentials {
	return &Credentials{
		gClient: gClient,
	}
}

func (c *Credentials) Remove(req router.Request, _ router.Response) error {
	creds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName()},
	})
	if err != nil {
		return err
	}

	for _, cred := range creds {
		if err := c.gClient.DeleteCredential(req.Ctx, req.Object.GetName(), cred.ToolName); err != nil {
			return err
		}
	}

	return nil
}
