# Bundled model data

When `MODEL_DATA_URL` is set, the Docker image build and the release build download the LLM
gateway's model prices and capabilities into this directory before compiling, and Obot embeds
them. CI sets it for the images and release binaries it publishes. To do it yourself:

    MODEL_DATA_URL=https://upgrade-server.obot.ai/v1/model-data go run ./tools/fetch-model-data

They're converted from LiteLLM's MIT-licensed model list (see `LICENSE.litellm`) and served at
`https://upgrade-server.obot.ai/v1/model-data`. The downloaded files (`pricing.json.gz`,
`model-parameters.json.gz` and `generated`) aren't committed. A build without them starts with
no model prices until Obot downloads them.
