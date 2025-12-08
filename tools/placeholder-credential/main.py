import os
import json
import asyncio
from gptscript import GPTScript, Options


if os.getenv("OBOT_RUN_ID") is not None:
	# This is being run by Obot. This credential being called means that a "global" credential has not been setup,
	# but that a user is trying to use it for their own Obot with their own credentials.
    exit(0)

if os.getenv("ENV_VARS") is None:
    print("No environment variables provided for prompting")
    exit(1)


async def main():
    g = GPTScript()
    env_vars = dict()

    for env in os.getenv("ENV_VARS").split(";"):
        out = await g.run("sys.prompt", Options(input=f'{{"message":"Please enter the value for {env}", "env":"{env}", "fields":"{env}","sensitive":"true"}}')).text()
        env_vars.update(json.loads(out))

    print(json.dumps({"env": env_vars}))

if __name__ == "__main__":
    asyncio.run(main())
