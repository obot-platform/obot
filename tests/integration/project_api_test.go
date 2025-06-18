package integration

import (
	"time"

	//revive:disable
	. "github.com/onsi/ginkgo/v2"
	//revive:disable
	. "github.com/onsi/gomega"
)

var _ = Describe("Project API", func() {
	var client *Client
	var createdID string

	BeforeEach(func() {
		client = NewClient("http://localhost:8080")
	})

	Context("When creating a new project", func() {
		It("should return 201 Created with a valid ID", func() {
			project, err := client.CreateProject()
			Expect(err).To(BeNil())

			Expect(project.ID).NotTo(BeEmpty())
			createdID = project.ID
		})

		It("should return 200 OK with correct project data", func() {
			// Ensure that a project was created
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
})
