package task

import (
	"errors"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrorValidating = errors.New("found existing external interface, only one external interface can be set")

type Handler struct {
}

func NewHandler() *Handler {
	return &Handler{}
}

func (t *Handler) HandleTaskCreationForCapabilities(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	if !thread.Spec.Project {
		return nil
	}

	if err := Validate(thread, false); err != nil {
		return err
	}

	if err := t.UpdateTaskForSlack(thread, req); err != nil {
		return err
	}

	if err := t.UpdateTaskForDiscord(thread, req); err != nil {
		return err
	}

	if err := t.UpdateTaskForEmail(thread, req); err != nil {
		return err
	}

	if err := t.UpdateTaskForWebhook(thread, req); err != nil {
		return err
	}

	return nil
}

func Validate(thread *v1.Thread, forSlack bool) error {
	if forSlack {
		if thread.Spec.Capabilities.OnDiscordMessage || thread.Spec.Capabilities.OnEmail != nil || thread.Spec.Capabilities.OnWebhook != nil {
			return ErrorValidating
		}
	}
	var count int
	if thread.Spec.Capabilities.OnSlackMessage {
		count++
	}
	if thread.Spec.Capabilities.OnDiscordMessage {
		count++
	}
	if thread.Spec.Capabilities.OnEmail != nil {
		if forSlack {
			count++
		}
		count++
	}
	if thread.Spec.Capabilities.OnWebhook != nil {
		count++
	}
	if count > 1 {
		return ErrorValidating
	}
	return nil
}

func (t *Handler) UpdateTaskForSlack(thread *v1.Thread, req router.Request) error {
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.SafeHashConcatName(system.WorkflowPrefix, "slack", thread.Name),
			Namespace: thread.Namespace,
		},
		Spec: v1.WorkflowSpec{
			ThreadName: thread.Name,
			Manifest: types.WorkflowManifest{
				OnSlackMessage: &types.TaskOnSlackMessage{},
				Steps: []types.Step{
					{
						Step: "Reply back to the user with a message in the thread",
					},
				},
				Name:        "Slack Integration Task",
				Description: "This task is used to integrate with Slack.",
			},
		},
	}

	if thread.Spec.Capabilities.OnSlackMessage {
		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			alias, err := randomtoken.Generate()
			if err != nil {
				return err
			}
			if len(alias) > 12 {
				alias = alias[:12]
			}
			workflow.Spec.Manifest.Alias = alias
			if err := req.Client.Create(req.Ctx, &workflow); err != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = workflow.Name
		}
	} else {
		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &workflow); kclient.IgnoreNotFound(err) != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = ""
		}
	}

	return nil
}

func (t *Handler) UpdateTaskForDiscord(thread *v1.Thread, req router.Request) error {
	workflowName := name.SafeHashConcatName(system.WorkflowPrefix, "discord", thread.Name)
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: thread.Namespace,
		},
		Spec: v1.WorkflowSpec{
			ThreadName: thread.Name,
			Manifest: types.WorkflowManifest{
				Steps: []types.Step{
					{
						Step: "Reply to the user with a message in the thread in discord",
					},
				},

				Name:             "Discord Integration Task",
				Description:      "This task is used to integrate with Discord.",
				OnDiscordMessage: &types.TaskOnDiscordMessage{},
			},
		},
	}

	if thread.Spec.Capabilities.OnDiscordMessage {
		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			alias, err := randomtoken.Generate()
			if err != nil {
				return err
			}
			if len(alias) > 12 {
				alias = alias[:12]
			}
			workflow.Spec.Manifest.Alias = alias
			if err := req.Client.Create(req.Ctx, &workflow); err != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = workflow.Name
		}
	} else {
		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &workflow); kclient.IgnoreNotFound(err) != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = ""
		}
	}

	return nil
}

func (t *Handler) UpdateTaskForEmail(thread *v1.Thread, req router.Request) error {
	workflowName := name.SafeHashConcatName(system.WorkflowPrefix, "email", thread.Name)
	emailReceiverName := name.SafeHashConcatName(system.EmailReceiverPrefix, workflowName)

	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: thread.Namespace,
		},
		Spec: v1.WorkflowSpec{
			ThreadName: thread.Name,
			Manifest: types.WorkflowManifest{
				Steps: []types.Step{
					{
						Step: "Inspect the email content and print the summary of the email",
					},
				},
				Name:        "Email Integration Task",
				Description: "This task is used to integrate with email.",
			},
		},
	}

	emailReceiver := v1.EmailReceiver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      emailReceiverName,
			Namespace: thread.Namespace,
		},
		Spec: v1.EmailReceiverSpec{
			EmailReceiverManifest: types.EmailReceiverManifest{
				Alias:        workflow.Spec.Manifest.Alias,
				WorkflowName: workflow.Name,
			},
			ThreadName: thread.Name,
		},
	}

	if thread.Spec.Capabilities.OnEmail != nil {
		emailReceiver.Spec.AllowedSenders = thread.Spec.Capabilities.OnEmail.AllowedSenders
		emailReceiver.Spec.Description = thread.Spec.Capabilities.OnEmail.Description
		emailReceiver.Spec.Name = thread.Spec.Capabilities.OnEmail.Name
		if err := req.Get(&emailReceiver, emailReceiver.Namespace, emailReceiver.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			if err := req.Client.Create(req.Ctx, &emailReceiver); err != nil {
				return err
			}
		}

		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			alias, err := randomtoken.Generate()
			if err != nil {
				return err
			}
			if len(alias) > 12 {
				alias = alias[:12]
			}
			workflow.Spec.Manifest.Alias = alias
			if err := req.Client.Create(req.Ctx, &workflow); err != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = workflow.Name
		}
	} else {
		if err := req.Get(&emailReceiver, emailReceiver.Namespace, emailReceiver.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &emailReceiver); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}

		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &workflow); kclient.IgnoreNotFound(err) != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = ""
		}
	}

	return nil
}

func (t *Handler) UpdateTaskForWebhook(thread *v1.Thread, req router.Request) error {
	workflowName := name.SafeHashConcatName(system.WorkflowPrefix, "webhook", thread.Name)
	webhookName := name.SafeHashConcatName(system.WebhookPrefix, workflowName)
	alias, err := randomtoken.Generate()
	if err != nil {
		return err
	}
	if len(alias) > 12 {
		alias = alias[:12]
	}

	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workflowName,
			Namespace: thread.Namespace,
		},
		Spec: v1.WorkflowSpec{
			ThreadName: thread.Name,
			Manifest: types.WorkflowManifest{
				Alias: alias,
				Steps: []types.Step{
					{
						Step: "Inspect the webhook content and print the summary of the webhook",
					},
				},
				Name:        "Webhook Integration Task",
				Description: "This task is used to integrate with webhook.",
			},
		},
	}

	webhook := v1.Webhook{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webhookName,
			Namespace: thread.Namespace,
		},
		Spec: v1.WebhookSpec{
			WebhookManifest: types.WebhookManifest{
				Alias:        alias,
				WorkflowName: workflowName,
			},
			ThreadName: thread.Name,
		},
	}

	if thread.Spec.Capabilities.OnWebhook != nil {
		webhook.Spec.Headers = thread.Spec.Capabilities.OnWebhook.Headers
		webhook.Spec.Secret = thread.Spec.Capabilities.OnWebhook.Secret
		webhook.Spec.ValidationHeader = thread.Spec.Capabilities.OnWebhook.ValidationHeader

		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			if err := req.Client.Create(req.Ctx, &workflow); err != nil {
				return err
			}

			thread.Status.WorkflowNameFromIntegration = workflow.Name
		}

		if err := req.Get(&webhook, webhook.Namespace, webhook.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err != nil {
			if err := req.Client.Create(req.Ctx, &webhook); err != nil {
				return err
			}
		}
	} else {
		if err := req.Get(&webhook, webhook.Namespace, webhook.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &webhook); kclient.IgnoreNotFound(err) != nil {
				return err
			}
		}

		if err := req.Get(&workflow, workflow.Namespace, workflow.Name); kclient.IgnoreNotFound(err) != nil {
			return err
		} else if err == nil {
			if err := req.Client.Delete(req.Ctx, &workflow); kclient.IgnoreNotFound(err) != nil {
				return err
			}
			thread.Status.WorkflowNameFromIntegration = ""
		}
	}

	return nil
}
