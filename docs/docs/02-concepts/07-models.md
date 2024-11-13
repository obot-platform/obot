# Models

A model is an AI program that has been trained on data to perform taks, recognize patterns, and make decisions. Popular examples include OpenAI's GPT-4o and Anthropic's Claude 3.5 Sonnet.

Otto8 comes with popular models for OpenAI preconfigured, but admins can modify them or create new ones. Agents, workflows, and tools can specify what model they should use. If they do not specify one, the system default model is used.

To create a model for a provider other than OpenAI, you must first enable the corresponding **Model Provider**. Otto8 currently supports three model providers:
- OpenAI
- Azure OpenAI
- Anthropic

You can learn more about how to configure Model providers in our [Model Provider Configuration Guide](/configuration/model-providers)