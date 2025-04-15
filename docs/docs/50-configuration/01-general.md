# General

The Obot server is configured via environment variables. The following configuration is available:

| Environment Variable | Description |
|---------------------|-------------|
| `OPENAI_API_KEY` | The foundation of Obot is a large language model that supports function-calling. The default is OpenAI and specifying an OpenAI key here will ensure none of the users need to worry about specifying their own API key. |
| `GITHUB_AUTH_TOKEN` | Obot and its underlying tool GPTScript make heavy use of tools hosted on GitHub. Care is taken to cache these tools and only re-check when necessary. However, rate-limiting can happen. Setting a read-only token here can alleviate many of these issues. |
| `OBOT_SERVER_DSN` | Obot uses a database backend. By default, it will use a sqlite3 local database. This environment variable allows you to specify another database option. For example, you can use a postgres database with something like `OBOT_SERVER_DSN=postgres://user:password@host/database`. |
| `OBOT_SERVER_HOSTNAME` | Tell Obot what its server URL is so that things like OAuth, LLM proxying, and invoke URLs are handled correctly. |
| `OBOT_SERVER_RETENTION_POLICY_HOURS` | The retention policy for the system. Set to 0 to disable retention. Default is 2160 (90 days) if left unset. This field should just be a number in a string, no `h` suffix. |

:::note

You can configure other [model-providers](02-model-providers.md) besides OpenAI.

:::
