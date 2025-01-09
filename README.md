# Obotasdfasdfaf:x


Obot is an open source AI agent platform. Key features include:
- Ability to build agents to support a variety of usecases including copilots, assistants, and autonomous agentic workflows.
- Integration with leading LLM providers
- Built-in RAG for your data
- Easy integration with custom or private web services and APIs
- OAuth 2.0 authentication

### Getting Started
Launch Obot via docker:
```bash
docker run -d -p 8080:8080 -e "OPENAI_API_KEY=<OPEN AI KEY>" ghcr.io/obot-platform/obot:latest
```
Then visit http://localhost:8080.

The `obot` CLI can be installed via brew on MacOS or Linux:
```bash
brew tap obot-platform/tap
brew install obot
```
or by downloading the binary for your platform from our [latest release](https://github.com/obot-platform/obot/releases/latest).

### Next Steps

For more information checkout our [Docs](https://docs.obot.ai/).
