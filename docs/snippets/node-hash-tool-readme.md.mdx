## Writing your first tool in Node.js (with Typescript)

[node-hash-tool](https://github.com/otto8-ai/node-hash-tool) contains a reference Typescript `Node.js` implementation of the `Hash` Tool.

This guide walks through the structure and design of the Tool and outlines the packaging requirements for [Otto8](https://docs.otto8.ai/concepts/agents)

To clone this repo and follow along, run the following command:

```bash
git clone git@github.com:otto8-ai/node-hash-tool
```
<br/>

## Tool Repo Structure

The directory tree below highlights the files required to implement `Hash` in Typescript and package it for `Otto8`.

```
node-hash-tool
├── package-lock.json
├── package.json
├── tsconfig.json
├── tool.gpt
└── src
    ├── hash.ts
    └── tools.ts
```

> **Note:** The `tsconfig.json` file is only required for tools written in Typescript.
> It is not necessary for tools written in JavaScript.

<br/>

## Defining the `Hash` Tool

The `tool.gpt` file contains [GPTScript Tool Definitions](https://docs.gptscript.ai/tools/gpt-file-reference) which describe a set of Tools that can be used by Agents in `Otto8`.
Every Tool repository must have a `tool.gpt` file in its root directory.

The Tools defined in this file must have a descriptive `Name` and `Description` that will help Agents understand what the Tool does, what it returns (if anything), and all the `Parameters` it takes.
Agents use these details to infer a Tool's usage.
We call the section of a Tool definition that contains this info a `Preamble`.

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
#!/usr/bin/env npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- hash
```

This is where the magic happens.

To oversimplify, when an Agent calls the `Hash` Tool, `Otto8` reads this line and then:

1. Downloads the appropriate `Node.js` tool chain (`node` and `npm`)
2. Sets up a working directory for the Tool
3. Installs the dependencies from the Tool's `package.json` and `package-lock.json`
4. Projects the call arguments onto environment variables (`DATA` and `ALGO`)
5. Runs `npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- hash`.

<br/>

Putting it all together, here's the complete definition of the `Hash` Tool.

```yaml
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Default is "sha256". Supports "sha256" and "md5".

#!/usr/bin/env npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- hash
```

<br/>

## Tool Metadata

The `tool.gpt` file also provides the following metadata for use in `Otto8`:

- `!metadata:*:category` which tags Tools with the `Crypto` category to promote organization and discovery
- `!metadata:*:icon` which assigns `https://cdn.jsdelivr.net/npm/@phosphor-icons/core@2/assets/duotone/fingerprint-duotone.svg` as the Tool icon

<br/>

> **Note:** `*` is a wild card pattern that applies the metadata to all Tools in the `tool.gpt` file.

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

#!/usr/bin/env npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- hash

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

As we saw earlier, the `npm` command invoked by the `Tool Body` passes `hash` as an argument to the `tool` script.

```bash
npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- hash
```

To figure out what this resolves to, let's inspect the `tool` script defined in `package.json`:

```json
  "scripts": {
    "tool": "node --no-warnings --loader ts-node/esm src/tools.ts"
  },
```

This means that when the `Tool Body` is executed, the effective command that runs is:

```bash
node --no-warnings --loader ts-node/esm src/tools.ts hash
```

> **Note:** The `--loader ts-node/esm` option, in conjunction with the contents of `tsconfig.json`, is the "special sauce" that lets us run Typescript code directly without transpiling it to JavaScript first.

To summarize, when the `Hash` Tool is called by an Agent, `src/tools.ts` gets run with `hash` as an argument.

Let's walk through the `src/tools.ts` to understand what happens at runtime:

```typescript
// ...
const cmd = process.argv[2]
try {
    switch (cmd) {
        case 'hash':
            console.log(hash(process.env.DATA, process.env.ALGO))
            break
        default:
            console.log(`Unknown command: ${cmd}`)
            process.exit(1)
    }

} catch (error) {
    // Print the error to stdout so that it can be captured by the GPTScript
    console.log(`${error}`)
    process.exit(1)
}
```

This code implements a simple CLI that wraps business logic in a try/catch block and forwards any exceptions to stdout. Writing errors to stdout instead of stderr is crucial because only stdout is returned to the Agent, while sdterr is discarded.

<details>
    <summary>
        <strong>Note:</strong> The simple CLI pattern showcased above is also easily extensible; adding business logic for new tools becomes a matter of adding a new case to the <code>switch</code> statement.
    </summary>

<br/>

For example, if we wanted to add a new Tool to verify a given hash, we'd add a `verify` case:

```typescript
switch (cmd) {
    case 'verify':
        console.log(verify(process.env.HASH, process.env.DATA, process.env.ALGO))
        break
    case 'hash':
        // ...
    default:
        // ...
    }
```

And the Body of the `Verify` Tool would pass `verify` to the `tool` script instead of `hash`:

```yaml
Name: Verify
# ...

#!/usr/bin/env npm --silent --prefix ${GPTSCRIPT_TOOL_DIR} run tool -- verify
```

</details>

<br/>

When `"hash"` is passed as an argument, the code extracts the `data` and `algo` Tool arguments from the respective environment variables, then passes them to the `hash` function.

The `hash` function is where the bulk of the business logic is implemented.

```typescript
import { createHash } from 'node:hash';

const SUPPORTED_ALGORITHMS = ['sha256', 'md5'];

export function hash(data: string = '', algo = 'sha256'): string {
  if (data === '') {
    throw new Error("A non-empty data argument must be provided");
  }

  if (!SUPPORTED_ALGORITHMS.includes(algo)) {
    throw new Error(`Unsupported hash algorithm ${algo} not in [${SUPPORTED_ALGORITHMS.join(', ')}]`);
  }

  return JSON.stringify({
    algo,
    hash: createHash(algo).update(data).digest('hex'),
  });
}
```

It starts off by validating the `data` and `algo` arguments.
When an argument is invalid, the function throws an exception that describes the validation issue in detail.
The goal is to provide useful information that an Agent can use to construct valid arguments for future calls.
For example, when an invalid `algo` argument is provided, the code returns an error that contains the complete list of valid algorithms.

Once it determines that all of the arguments are valid, it calculates the hash and writes a JSON object to stdout.
This object contains the hash and the algorithm used to generate it.

```typescript
  // ...
  return JSON.stringify({
    algo,
    hash: createHash(algo).update(data).digest('hex'),
  });
```

> **Note:** Producing structured data with extra contextual info (e.g. the algorithm) is considered good form.
> It's a pattern that improves the Agent's ability to correctly use the Tool's result over time.

<details>
    <summary>
    Complete <code>package.json</code>, <code>src/tools.ts</code>, and <code>src/hash.ts</code>
    </summary>

```json
{
  "type": "module",
  "scripts": {
    "tool": "node --no-warnings --loader ts-node/esm src/tools.ts"
  },
  "dependencies": {
    "@types/node": "^20.16.11",
    "ts-node": "^10.9.2",
    "typescript": "^5.4.5"
  },
  "devDependencies": {}
}
```

```typescript
// src/tools.ts
import { hash } from './hash.ts'

if (process.argv.length !== 3) {
    console.error('Usage: node tool.ts <command>')
    process.exit(1)
}

const cmd = process.argv[2]
try {
    switch (cmd) {
        case 'hash':
            console.log(hash(process.env.DATA, process.env.ALGO))
            break
        default:
            console.log(`Unknown command: ${cmd}`)
            process.exit(1)
    }

} catch (error) {
    // Print the error to stdout so that it can be captured by the GPTScript
    console.log(`${error}`)
    process.exit(1)
}
```

```typescript
// src/hash.ts
import { createHash } from 'node:hash';

const SUPPORTED_ALGORITHMS = ['sha256', 'md5'];

export function hash(data: string = '', algo = 'sha256'): string {
  if (data === '') {
    throw new Error("A non-empty data argument must be provided");
  }

  if (!SUPPORTED_ALGORITHMS.includes(algo)) {
    throw new Error(`Unsupported hash algorithm ${algo} not in [${SUPPORTED_ALGORITHMS.join(', ')}]`);
  }

  return JSON.stringify({
    algo,
    hash: createHash(algo).update(data).digest('hex'),
  });
}
```
</details>

<br/>

## Testing `src/tools.ts` and `src/hash.ts` Locally

Before adding a Tool to `Otto8`, verify that the Typescript business logic works on your machine.

To do this, run through the following steps in the root of your local fork:

1. Install dependencies

   ```bash
   npm install
   ```

2. Run the Tool with some test arguments:

   | **Command**                                        |**Output**                                                                                              |
   | -------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
   | `DATA='foo' npm run tool hash`                  | `{ "algo": "sha256", "hash": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae" }` |
   | `npm run tool hash`                             | `Error: A data argument must be provided`                                                          |
   | `DATA='foo' ALGO='md5' npm run tool hash`       | `{ "algo": "md5", "hash": "acbd18db4cc2f85cedef654fccc4a4d8" }`                                    |
   | `DATA='foo' ALGO='whirlpool' npm run tool hash` | `Error: Unsupported hash algorithm: whirlpool not in ['sha256', 'md5']`                            |

<br/>

## Adding The `Hash` Tool to `Otto8`

Before a Tool can be used by an Agent, an admin must first add the Tool to `Otto8` by performing the steps below:

1. <details>
       <summary>Navigate to the <code>Otto8</code> admin UI in a browser and open the Tools page by clicking the <em>Tools</em> button in the left drawer</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/add-tools-step-0.png"
                alt="Open The Tools Page" width="200"/>
       </div>
   </details>

2. <details>
       <summary>Click the <em>Register New Tool</em> button on the right</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/add-tools-step-1.png"
                alt="Click The Register New Tool Button" width="200"/>
       </div>
   </details>

3. <details>
       <summary>Type the Tool repo reference into the modal's input box and click <em>Register Tool</em></summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/add-tools-step-2.png"
                alt="Enter Tool Repo Reference" width="500" height="auto"/>
       </div>
   </details>

<br/>

<details>
    <summary>Once the tool has been added, you can search for it by category or name on the Tools page to verify</summary>
    <div align="left">
        <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/add-tools-step-3.png"
             alt="Search For Newly Added Tools" height="300"/>
    </div>
</details>

## Using The `Hash` Tool in an Agent

To use the `Hash` Tool in an Agent, open the Agent's Edit page, then:

1. <details>
       <summary>Click the <em>Add Tool</em> button under either the <em>Agent Tools</em> or <em>User Tools</em> sections</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/use-tools-step-0.png"
                alt="Click The Add Tool Button" width="500"/>
       </div>
   </details>

2. <details>
       <summary>Search for "Hash" or "Crypto" in the Tool search pop-out and select the <code>Hash</code> Tool</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/use-tools-step-1.png"
                alt="Add Hash Tool To Agent" width="500"/>
       </div>
   </details>

3. <details>
       <summary>Ask the Agent to generate a hash</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/node-hash-tool/refs/heads/main/docs/use-tools-step-2.png"
                alt="Ask The Agent To Generate a Hash" width="500"/>
       </div>
   </details>
