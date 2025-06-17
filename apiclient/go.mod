module github.com/obot-platform/obot/apiclient

go 1.23.1

replace github.com/danielgtaylor/huma/v2 => github.com/gptscript-ai/huma v0.0.0-20250617131016-b2081da6c65b

require (
	github.com/danielgtaylor/huma/v2 v2.32.1-0.20250509235652-c7ead6f3c67f
	github.com/gptscript-ai/go-gptscript v0.9.6-0.20250617131750-9129819aea51
	github.com/obot-platform/obot/logger v0.0.0-20241217130503-4004a5c69f32
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/sys v0.33.0 // indirect
)
