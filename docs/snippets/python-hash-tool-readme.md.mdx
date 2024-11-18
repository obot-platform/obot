## Writing your first tool in Python

[python-hash-tool](https://github.com/otto8-ai/python-hash-tool) contains a reference `Python` implementation of the `Hash` Tool.

This guide walks through the structure and design of the Tool and outlines the packaging requirements for [Otto8](https://docs.otto8.ai/concepts/agents)

To clone this repo and follow along, run the following command:

```bash
git clone git@github.com:otto8-ai/python-hash-tool
```

<br/>

## Tool Repo Structure

The directory tree below highlights the files required to implement `Hash` in Python and package it for `Otto8`.

```
python-hash-tool
├── hash.py
├── requirements.txt
└── tool.gpt
```
<br/>

## Defining the `Hash` Tool

The `tool.gpt` file contains [GPTScript Tool Definitions](https://docs.gptscript.ai/tools/gpt-file-reference) which describe a set of Tools that can be used by Agents in `Otto8`.
Every Tool repository must have a `tool.gpt` file in its root directory.

The Tools defined in this file must have a descriptive `Name` and `Description` that will help Agents understand what the Tool does, what it returns (if anything), and all the `Parameters` it takes.
Agents use these details to figure out when and how to use the Tool. We call the section of a Tool definition that contains this info a `Preamble`.

We want the `Hash` Tool to return the hash of some given `data`. It would also be nice to support a few different algorithms for the Agent to choose from.

Let's take a look at the `Preamble` for `Hash` to see how that's achieved:

```yaml
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

<br/>

Immediately below the `Preamble` is the `Tool Body`, which tells `Otto8` how to execute the Tool:

```bash
 #!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/hash.py
```


This is where the magic happens.

To oversimplify, when an Agent calls the `Hash` Tool, `Otto8` reads this line and then:

1. Downloads the appropriate `Python` tool chain
2. Sets up a working directory for the Tool and creates a virtual environment
3. Installs the dependencies from the `requirements.txt`, if present
4. Projects the call arguments onto environment variables (`DATA` and `ALGO`)
5. Runs `python3 ${GPTSCRIPT_TOOL_DIR}/hash.py`.

<br/>

Putting it all together, here's the complete definition of the `Hash` Tool.

```yaml
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Default is "sha256". Supports "sha256" and "md5".

#!/usr/bin/env python3 ${GPTSCRIPT_TOOL_DIR}/hash.py
```

<br/>

## Tool Metadata

The `tool.gpt` file also provides the following metadata for use in `Otto8`:

- `!metadata:*:category` which tags Tools with the `Crypto` category to promote organization and discovery
- `!metadata:*:icon` which assigns `https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg` as the Tool icon

<br/>

> **Note:** `*` is a wild card pattern that applies the metadata to all Tools in a `tool.gpt`.

```yaml
---
!metadata:*:category
Crypto

---
!metadata:*:icon
https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg
```

<details>
    <summary>
    <strong>Note:</strong> Metadata can be applied to a specific Tool by either specifying the exact name (e.g. <code>!metadata:Hash:category</code>) or by adding the metadata directly to a Tool's <code>Preamble</code>
    </summary>

```yaml
Name: Hash
Metadata: category: Crypto
Metadata: icon: https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg
```
</details>

<br/>

<details>
    <summary>Complete <code>tool.gpt</code></summary>

```yaml
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

</details>

<br/>

## Implementing Business Logic

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

> **Note:** Producing structured data with extra contextual info (e.g. the algorithm) is considered good form.
> It's a pattern that improves the Agent's ability to correctly use the Tool's result over time.

<details>
    <summary>Complete <code>hash.py</code></summary>

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

</details>

<br/>

## Testing `hash.py` Locally

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

<br/>

## Adding The `Hash` Tool to `Otto8`

Before a Tool can be used by an Agent, an admin must first add the Tool to `Otto8` by performing the steps below:

1. <details>
       <summary>Navigate to the <code>Otto8</code> admin UI in a browser and open the Tools page by clicking the <em>Tools</em> button in the left drawer</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-0.png"
                alt="Open The Tools Page" width="200"/>
       </div>
   </details>

2. <details>
       <summary>Click the <em>Register New Tool</em> button on the right</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-1.png"
                alt="Click The Register New Tool Button" width="200"/>
       </div>
   </details>

3. <details>
       <summary>Type the Tool repo reference into the modal's input box and click <em>Register Tool</em></summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-2.png"
                alt="Enter Tool Repo Reference" width="500" height="auto"/>
       </div>
   </details>

<br/>

<details>
    <summary>Once the tool has been added, you can search for it by category or name on the Tools page to verify</summary>
    <div align="left">
        <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/add-tools-step-3.png"
             alt="Search For Newly Added Tools" height="300"/>
    </div>
</details>

## Using The `Hash` Tool in an Agent

To use the `Hash` Tool in an Agent, open the Agent's Edit page, then:

1. <details>
       <summary>Click the <em>Add Tool</em> button under either the <em>Agent Tools</em> or <em>User Tools</em> sections</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-0.png"
                alt="Click The Add Tool Button" width="500"/>
       </div>
   </details>

2. <details>
       <summary>Search for "Hash" or "Crypto" in the Tool search pop-out and select the <code>Hash</code> Tool</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-1.png"
                alt="Add Hash Tool To Agent" width="500"/>
       </div>
   </details>

3. <details>
       <summary>Ask the Agent to generate a hash</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/python-hash-tool/refs/heads/main/docs/use-tools-step-2.png"
                alt="Ask The Agent To Generate a Hash" width="500"/>
       </div>
   </details>
