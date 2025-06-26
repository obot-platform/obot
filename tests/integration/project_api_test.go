// revive:disable:dot-imports

package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	client    *Client
	createdID string
)

var _ = BeforeSuite(func() {
	client = NewClient("http://localhost:8080")

	agent, err := client.GetAgent("a1-obot")
	Expect(err).To(BeNil())

	models, err := client.GetModels()
	Expect(err).To(BeNil())

	for _, model := range models {
		if model.ModelProvider == "mock-model-provider" {
			err = client.SetUpDefaultModelAlias(model.ID, "llm")
			Expect(err).To(BeNil())

			agent.Model = model.ID
			agent, err = client.UpdateAgent(agent.ID, *agent)
			Expect(err).To(BeNil())
		}
	}
})

var _ = Describe("Project API", Ordered, func() {
	Context("When creating a new project", func() {
		It("should return 201 Created with a valid ID", func() {
			project, err := client.CreateProject()
			Expect(err).To(BeNil())

			Expect(project.ID).NotTo(BeEmpty())
			createdID = project.ID
		})

		It("should return 200 OK with correct project data", func() {
			Expect(createdID).NotTo(BeEmpty())

			project, err := client.GetProject(createdID)
			Expect(err).To(BeNil())

			Expect(project.ID).To(Equal(createdID))
		})
	})

	Context("When configuring Slack Integration	for the created project", func() {
		It("should return 200 OK when Slack config is valid", func() {
			Expect(createdID).NotTo(BeEmpty())

			slackReceiver, err := client.ConfigureProjectSlack(createdID, map[string]interface{}{
				"appId":         "foo",
				"clientId":      "foo",
				"clientSecret":  "foo",
				"signingSecret": "foo",
			})
			Expect(err).To(BeNil())

			Expect(slackReceiver.AppID).To(Equal("foo"))
			Expect(slackReceiver.ClientID).To(Equal("foo"))
		})

		It("should eventually set task name into project", func() {
			Expect(createdID).NotTo(BeEmpty())

			Eventually(func(g Gomega) {
				project, err := client.GetProject(createdID)
				g.Expect(err).To(BeNil())

				g.Expect(project.Capabilities.OnSlackMessage).To(BeTrue())
				g.Expect(project.WorkflowNamesFromIntegration.SlackWorkflowName).NotTo(BeEmpty())

				slackWorkflowName := project.WorkflowNamesFromIntegration.SlackWorkflowName
				task, err := client.GetProjectTask(createdID, slackWorkflowName)
				Expect(err).To(BeNil())

				Expect(task.ID).To(Equal(slackWorkflowName))
				Expect(task.ProjectID).To(Equal(createdID))
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond)
		})
	})

	Context("When creating a new thread", func() {
		var threadID string
		It("should return 201 Created with a valid ID", func() {
			Expect(createdID).NotTo(BeEmpty())

			thread, err := client.CreateThread(createdID)
			Expect(err).To(BeNil())

			Expect(thread.ID).NotTo(BeEmpty())
			Expect(thread.ProjectID).To(Equal(createdID))

			thread, err = client.GetProjectThread(createdID, thread.ID)
			Expect(err).To(BeNil())

			threadID = thread.ID
		})

		It("can invoke the thread and get the result", func() {
			Expect(threadID).NotTo(BeEmpty())

			err := client.InvokeProjectThread(createdID, threadID, "Hello, world!")
			Expect(err).To(BeNil())

			messages, _ := client.GetProjectThreadEventsStream(createdID, threadID)

			for message := range messages {
				if message.Data.Input != "" {
					Expect(message.Data.Input).To(Equal("Hello, world!"))
				} else if message.Data.Content != "" {
					Expect(message.Data.Content).To(Equal("This is a fake response for testing."))
				}
				if message.Data.RunComplete {
					break
				}
			}
		})

	})
})
