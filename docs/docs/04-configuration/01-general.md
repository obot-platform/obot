# General

The Otto8 server is configured via environment variables. The following configuration is available:

#### `OPENAI_API_KEY`

The foundation of Otto8 is a large language model that supports function-calling. The default is OpenAI and specifying an OpenAI key here will ensure none of the users need to worry about specifying their own API key.

:::note

You can configure other [model-providers](02-model-providers.md) besides OpenAI.

:::

#### `GITHUB_AUTH_TOKEN` 

Otto8 and its underlying tool GPTScript make heavy use of tools hosted on GitHub. Care is taken to cache these tools and only re-check when necessary. However, rate-limiting can happen. Setting a read-only token here can alleviate many of these issues.

#### `OTTO8_SERVER_DSN`

Otto8 uses a database backend. By default, it will use a sqlite3 local database. This environment variable allows you to specify another database option. For example, you can use a postgres database with something like `OTTO8_SERVER_DSN=postgres://user:password@host/database`.

#### `OTTO8_SERVER_HOSTNAME`

Tell Otto8 what its server URL is so that things like OAuth, LLM proxying, and invoke URLs are handled correctly.
