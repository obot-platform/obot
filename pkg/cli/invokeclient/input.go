package invokeclient

import (
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/textio"
)

type QuietInputter struct {
}

func (d QuietInputter) Next(previous string, resp *types.InvokeResponse) (string, bool, error) {
	if resp == nil {
		return previous, true, nil
	}
	return "", false, nil
}

type VerboseInputter struct {
	client *apiclient.Client
}

func nextInput() (string, bool, error) {
	x, err := textio.Ask("Input", "")
	if err != nil {
		return "", false, err
	}
	return x, true, nil
}

func (d VerboseInputter) Next(previous string, resp *types.InvokeResponse) (string, bool, error) {
	if resp == nil {
		if previous == "" {
			return nextInput()
		}
		return previous, true, nil
	}

	return nextInput()
}
