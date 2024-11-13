## Writing your first tool in Python

[python-hash-tool](https://github.com/otto8-ai/python-hash-tool) contains a reference `Python` implementation of the `Hash` Tool.

This guide walks through the structure and design of the Tool and outlines the packaging requirements for [Otto8](https://docs.otto8.ai/concepts/agents)

To clone this repo and follow along, run the following command:

```bash
git clone git@github.com:otto8-ai/python-hash-tool
```

---

### Tool Repo Structure

The directory tree below highlights the files required to implement `Hash` in Python and package it for `Otto8`.

```
python-hash-tool
├── hash.py
├── requirements.txt
└── tool.gpt
```

---

### Defining the `Hash` Tool

The `tool.gpt` file contains [GPTScript Tool Definitions](https://docs.gptscript.ai/tools/gpt-file-reference) which describe a set of Tools that
can be used by Agents in `Otto8`.
Every Tool repository must have a `tool.gpt` file in its root directory.

The Tools defined in this file must have a descriptive `Name` and `Description` that will help Agents understand what the Tool does, what it returns (if anything), and all the `Parameters` it takes.
Agents use these details to figure out when and how to use the Tool. We call the section of a Tool definition that contains this info a `Preamble`.

We want the `Hash` Tool to return the hash of some given `data`. It would also be nice to support a few different algorithms for the Agent to choose from.
Let's take a look at the `Preamble` for `Hash` to see how that's achieved:

```text
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Supports "sha256" and "md5". Default is "sha256"
```

Breaking this down a bit:

- The `Preamble` above declares a Tool named `Hash`.
- The `Param` fields enumerate the arguments that an Agent must provide when calling `Hash`, `data` and `algo`.
- In this case, the description of the `algo` parameter outlines the valid options (`sha256` or `md5`) and defines a default value (`sha256`)
- The `Description` explains what `Hash` returns with respect to the given arguments; the hash of `data` using the algorithm selected with `algo`.

Immediately below the `Preamble` is the `Tool Body`, which tells `Otto8` how to execute the Tool:

```text
 #!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/hash.py
```

This is where the magic happens.

To oversimplify, when an Agent calls the `Hash` Tool, `Otto8` reads this line and then:

1. Downloads the appropriate `Python` tool chain
2. Sets up a working directory for the Tool and creates a virtual environment
3. Installs the dependencies from the `requirements.txt`, if present
4. Projects the call arguments onto environment variables (`DATA` and `ALGO`)
5. Runs `python3 ${GPTSCRIPT_TOOL_DIR}/hash.py`.

Putting it all together, here's the complete definition of the `Hash` Tool.

```text
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Default is "sha256". Supports "sha256" and "md5".

#!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/hash.py
```

### Tool Metadata

The `tool.gpt` file also provides the following metadata for use in `Otto8`:

- `!metadata:*:category` which tags Tools with the `Crypto` category to promote organization and discovery
- `!metadata:*:icon` which assigns `https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg` as the Tool icon

Where `*` is a wild card pattern that applies the metadata to all Tools in a `tool.gpt`.

```text
---
!metadata:*:category
Crypto

---
!metadata:*:icon
https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg
```

**Note:** Metadata can be applied to a specific Tool by either specifying the exact name (e.g. `!metadata:Hash:category`) or by adding the metadata directly to a Tool's `Preamble`:

```text
Name: Hash
Metadata: category: Crypto
Metadata: icon: https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg
```

### Complete `tool.gpt`

```text
---
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Supports "sha256" and "md5". Default is "sha256"

#!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/hash.py

---
!metadata:*:category
Crypto

---
!metadata:*:icon
https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg
```

---

### Implementing Business Logic

The `hash.py` file executed by the `Tool Body` is the concrete implementation of the Tool's business logic.

Let's walk through the code to understand how it works.

```python
if __name__ == '__main__':
    try:
        main()
    except Exception as err:
        # Print err to stdout to return the error to the agent
        print(f'Error: {err}')
        sys.exit(1)
```

Starting at the bottom, the `main` function is called in a `try` block so that any runtime exceptions caught are written to stdout.
This is important because everything written to stdout is returned to the Agent when the Tool call is completed, while everything written to stderr is discarded.
Using this pattern ensures that when a Tool call fails, the calling Agent is informed of the failure.

Moving on, the `main` function implements the meat and potatoes of the `Hash` Tool.

```python
SUPPORTED_HASH_ALGORITHMS = ['sha256', 'md5']

def main():
    # Extract the tool's `data` argument from the env
    data = os.getenv('DATA')
    if not data:
        raise ValueError('A data argument must be provided')

    # Extract the tool's `algo` argument from the env and default to `sha256`
    algo = os.getenv('ALGO', 'sha256')
    if algo not in SUPPORTED_HASH_ALGORITHMS:
        # Return the supported algorithms in the error message to help agents choose a valid
        # algorithm the next time they call this tool
        raise ValueError(f'Unsupported hash algorithm: {algo} not in {SUPPORTED_HASH_ALGORITHMS}')
    #...
```

It starts off by extracting the Tool's arguments from the respective environment variables and validates them.
When an argument is invalid, the function raises an exception that describes the validation issue in detail.
The goal is to provide useful information that an Agent can use to construct valid arguments for future calls.
For example, when an invalid `algo` argument is provided, the code returns an error that contains the complete list of valid algorithms.

After validating the Tool arguments, it calculates the hash and writes a JSON object to stdout.
This object contains the hash and the algorithm used to generate it.

```python
    # ...
    # Generate the hash
    hash_obj = hashlib.new(algo)
    hash_obj.update(data.encode('utf-8'))

    # Return the hash along with the algorithm used to generate it.
    # Providing more information in the tool's response makes it easier for agents to keep
    # track of the context.
    print(json.dumps({
        'algo': algo,
        'hash': hash_obj.hexdigest()
    }))
```

Producing structured data with extra contextual info (e.g. the algorithm) is considered good form.
It's a pattern that improves the Agent's ability to correctly use the Tool's result over time.

### Complete `hash.py`

```python
import hashlib
import json
import os
import sys

SUPPORTED_HASH_ALGORITHMS = ['sha256', 'md5']


def main():
    # Extract the tool's `data` argument from the env
    data = os.getenv('DATA')
    if not data:
        raise ValueError('A data argument must be provided')

    # Extract the tool's `algo` argument from the env and default to `sha256`
    algo = os.getenv('ALGO', 'sha256')
    if algo not in SUPPORTED_HASH_ALGORITHMS:
        # Return the supported algorithms in the error message to help assistants choose a valid
        # algorithm the next time they call this tool
        raise ValueError(f'Unsupported hash algorithm: {algo} not in {SUPPORTED_HASH_ALGORITHMS}')

    # Generate the hash
    hash_obj = hashlib.new(algo)
    hash_obj.update(data.encode('utf-8'))

    # Return the hash along with the algorithm used to generate it.
    # Providing more information in the tool's response makes it easier for assistants to keep
    # track of the context.
    print(json.dumps({
        'algo': algo,
        'hash': hash_obj.hexdigest()
    }))


if __name__ == '__main__':
    try:
        main()
    except Exception as err:
        # Print err to stdout to return the error to the assistant
        print(f'Error: {err}')
        sys.exit(1)
```

---

### Testing `hash.py` Locally

Before adding a Tool to `Otto8`, verify that the Python business logic works on your machine.

To do this, run through the following steps in the root of your local fork:

1. Set up a virtual environment:

   ```bash
   python3 -m venv venv
   source venv/bin/activate
   ```

2. Activate the virtual environment:

   ```bash
   source venv/bin/activate
   ```

3. Install and freeze dependencies:

   ```bash
   pip install -r requirements.txt
   pip freeze > requirements.txt
   ```

4. Run the Tool with some test arguments:

   | **Command**                                        | **Output**                                                                                              |
   | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
   | `DATA='foo' python3 hash.py`                  | `{ "algo": "sha256", "hash": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae" }` |
   | `python3 hash.py`                             | `Error: A data argument must be provided`                                                          |
   | `DATA='foo' ALGO='md5' python3 hash.py`       | `{ "algo": "md5", "hash": "acbd18db4cc2f85cedef654fccc4a4d8" }`                                    |
   | `DATA='foo' ALGO='whirlpool' python3 hash.py` | `Error: Unsupported hash algorithm: whirlpool not in ['sha256', 'md5']`                            |

---

### Adding The `Hash` Tool to `Otto8`

Before a Tool can be used by an Agent, an admin must first add the Tool to `Otto8` by performing the steps below:

1. Navigate to the `Otto8` admin UI in a browser and open the Tools page by clicking the "Tools" button in the left drawer
   ![Open The Tools Page](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-0.png "Open The Tools Page")

2. Click the "Register New Tool" button on the right
   ![Click The Register New Tool Button](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-1.png "Click The Register New Tool Button")

3. Type the Tool repo reference into the modal's input box -- in this example `github.com/otto8-ai/python-hash-tool` -- and click "Register Tool"
   ![Enter Tool Repo Reference](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-2.png "Enter Tool Repo Reference")

Afterwords, the Tool will be available for use in `Otto8`.

You can search for the Tool by category or name on the Tools page to verify:

![Search For Newly Added Tools](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-3.png "Search For Newly Added Tools")

### Using The `Hash` Tool in an Agent

To use the `Hash` Tool in an Agent, open the Agent's Edit page, then:

1. Click the "Add Tool" button under either the "Agent Tools" or "User Tools" sections
   ![Click The Add Tool Button](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-0.png "Click The Add Tool Button")

2. Search for "Hash" or "Crypto" in the Tool search pop-out and select the `Hash` Tool
   ![Add Hash Tool To Agent](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-1.png "Add Hash Tool To Agent")

3. Ask the Agent to generate a hash
   ![Ask The Agent To Generate a Hash](https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-2.png "Ask The Agent To Generate a Hash")
