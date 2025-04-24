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
	localCreds, err := c.gClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{req.Object.GetName() + "-local"},
	})
	if err != nil {
		return err
	}

	for _, cred := range append(creds, localCreds...) {
		if err := c.gClient.DeleteCredential(req.Ctx, cred.Context, cred.ToolName); err != nil {
			return err
		}
	}

	return nil
}
