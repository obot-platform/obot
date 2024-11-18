## Writing your first tool in Go

[go-hash-tool](https://github.com/otto8-ai/go-hash-tool) contains a reference `Go` implementation of the `Hash` Tool.

This guide walks through the structure and design of the Tool and outlines the packaging requirements for [Otto8](https://docs.otto8.ai/concepts/agents)

To clone this repo and follow along, run the following command:

```bash
git clone git@github.com:otto8-ai/go-hash-tool
```

<br/>

## Tool Repo Structure

The directory tree below highlights the files required to implement `Hash` in Go and package it for `Otto8`.

```
go-hash-tool
├── tool.gpt
├── go.mod
├── main.go
└── commands
    └── hash.go
```

> **Note:** Most Tools implemented in Go will also have a `go.sum` file that is also required when present.
> It is not present in the reference implementation because it has no external dependencies and relies solely on the Go standard library.

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
#!{GPTSCRIPT_TOOL_DIR}/bin/gptscript-go-tool hash
```

This is where the magic happens.

To oversimplify, when an Agent calls the `Hash` Tool, `Otto8` reads this line and then:

1. Downloads the appropriate `Go` tool chain
2. Sets up a working directory for the Tool
3. Runs `go build` to install dependencies (from `go.mod` and `go.sum`) and build a binary named `gptscript-go-tool` (`gptscript-go-tool.exe` on Windows)
4. Projects the call arguments onto environment variables (`DATA` and `ALGO`)
5. Runs `gptscript-go-tool hash`.

<br/>

Putting it all together, here's the complete definition of the `Hash` Tool:

```yaml
Name: Hash
Description: Generate a hash of data using the given algorithm and return the result as a hexadecimal string
Param: data: The data to hash
Param: algo: The algorithm to generate a hash with. Default is "sha256". Supports "sha256" and "md5".

#!{GPTSCRIPT_TOOL_DIR}/bin/gptscript-go-tool hash
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

#!{GPTSCRIPT_TOOL_DIR}/bin/gptscript-go-tool hash

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

The `main.go` file is the entry point of the `gptscript-go-tool` binary that is executed by `Otto8` when the `Hash` Tool is called.

Let's walk through the the code to understand what happens at runtime:

```go
// ...
switch cmd := os.Args[0]; cmd {
case "hash":
    res, err = commands.Hash(os.Getenv("DATA"), os.Getenv("ALGO"))
default:
    err = fmt.Errorf("Unsupported command: %s", cmd)
}

if err != nil {
    fmt.Println(err)
    os.Exit(1)
}

if res != "" {
    fmt.Println(res)
}
```

This code implements a simple CLI responsible for dispatching the `commands.Hash` function on request -- when `hash` is passed in as an argument -- after extracting the Tool arguments, `data` and `algo`, from the respective environment variables.

It also ensures that the return value and errors of the call to `commands.Hash` are written to stdout. This is crucial because only stdout is returned to the Agent, while stderr is discarded.

<details>
    <summary>
        <strong>Note:</strong> The simple CLI pattern showcased above is also easily extensible; adding business logic for new tools becomes a matter of adding a new case to the <code>switch</code> statement.
    </summary>

<br/>

For example, to add business logic for a new Tool to verify a hash, we just have to tack on `verify` case:

```go
// ...
case "verify":
     res, err = commands.Verify(os.Getenv("HASH"), os.Getenv("DATA"), os.Getenv("ALGO"))
case "hash":
    // ...
default:
    //...
```

The Body of the `Verify` Tool definition would then simply pass `verify` to `gptscript-go-tool` instead of `hash`:

```yaml
Name: Verify
# ...

#!{GPTSCRIPT_TOOL_DIR}/bin/gptscript-go-tool verify
```

</details>

<br/>

The `commands.Hash` function implements the bulk of the `Hash` Tool's business logic.

It starts off by validating the `data` and `algo` arguments.


```go
func Hash(data, algo string) (string, error) {
    if data == "" {
        return "", fmt.Errorf("A non-empty data argument must be provided")
    }

    if algo == "" {
        algo = "sha256"
    }

    sum, ok := hashFunctions[algo]
	if !ok {
		return "", fmt.Errorf("Unsupported hash algorithm: %s not in [%s]", algo, hashFunctions)
	}
    // ...
```

When an argument is invalid, the function returns an error that describes the validation issue in detail.
The goal is to provide useful information that an Agent can use to construct valid arguments for future calls.
For example, when an invalid `algo` argument is provided, the code returns an error that contains the complete list of valid algorithms.

<br/>

Once it determines that all of the arguments are valid, it then calculates the hash and writes a JSON object to stdout.
This object contains both the hash and the algorithm used to generate it.

```go
    // ...
    hash, err := json.Marshal(hashResult{
		Algo: algo,
		Hash: hex.EncodeToString(sum([]byte(data))),
	})
	if err != nil {
		return "", fmt.Errorf("Failed to marshal hash result: %w", err)
	}

	return string(hash), nil
}
```

> **Note:** Producing structured data with extra contextual info (e.g. the algorithm) is considered good form.
> It's a pattern that improves the Agent's ability to correctly use the Tool's result over time.

<details>
    <summary>Complete <code>main.go</code> and <code>hash.go</code></summary>

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/otto8-ai/go-hash-tool/commands"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: gptscript-go-tool <command>")
		os.Exit(1)
	}

	var (
		err error
		res string
	)
	switch cmd := os.Args[1]; cmd {
	case "hash":
		res, err = commands.Hash(os.Getenv("DATA"), os.Getenv("ALGO"))
	default:
		err = fmt.Errorf("Unsupported command: %s", cmd)
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if res != "" {
		fmt.Println(res)
	}
}
```

```go
// commands/hash.go
package commands

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func Hash(data, algo string) (string, error) {
	if data == "" {
		return "", fmt.Errorf("A non-empty data argument must be provided")
	}

	if algo == "" {
		algo = "sha256"
	}

	sum, ok := hashFunctions[algo]
	if !ok {
		return "", fmt.Errorf("Unsupported hash algorithm: %s not in [%s]", algo, hashFunctions)
	}

	hash, err := json.Marshal(hashResult{
		Algo: algo,
		Hash: hex.EncodeToString(sum([]byte(data))),
	})
	if err != nil {
		return "", fmt.Errorf("Failed to marshal hash result: %w", err)
	}

	return string(hash), nil
}

type hashResult struct {
	Algo string `json:"algo"`
	Hash string `json:"hash"`
}

var hashFunctions = hashFuncSet{
	"sha256": func(d []byte) []byte { h := sha256.Sum256(d); return h[:] },
	"md5":    func(d []byte) []byte { h := md5.Sum(d); return h[:] },
}

type hashFuncSet map[string]func([]byte) []byte

func (s hashFuncSet) String() string {
	return strings.Join(keys(s), ", ")
}

func keys[V any](m map[string]V) []string {
	set := make([]string, 0, len(m))
	for k := range m {
		set = append(set, k)
	}

	sort.Strings(set)
	return set
}
```

</details>

<br/>

## Testing `main.go` Locally

Before adding a Tool to `Otto8`, verify that the Go business logic works on your machine.

To do this, run through the following steps in the root of your local fork:

1. Install dependencies and build the binary

   ```bash
   make build
   ```

2. Run the Tool with some test arguments:

   | **Command**                                              | **Output**                                                                                         |
   | -------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
   | `DATA='foo' bin/gptscript-go-tool hash`                  | `{ "algo": "sha256", "hash": "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae" }` |
   | `DATA='' bin/gptscript-go-tool hash`                     | `Error: A data argument must be provided`                                                          |
   | `DATA='foo' ALGO='md5' bin/gptscript-go-tool hash`       | `{ "algo": "md5", "hash": "acbd18db4cc2f85cedef654fccc4a4d8" }`                                    |
   | `DATA='foo' ALGO='whirlpool' bin/gptscript-go-tool hash` | `Error: Unsupported hash algorithm: whirlpool not in ['sha256', 'md5']`                            |

<br/>

## Adding The `Hash` Tool to `Otto8`

Before a Tool can be used by an Agent, an admin must first add the Tool to `Otto8` by performing the steps below:

1. <details>
       <summary>Navigate to the <code>Otto8</code> admin UI in a browser and open the Tools page by clicking the <em>Tools</em> button in the left drawer</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/add-tools-step-0.png"
                alt="Open The Tools Page" width="200"/>
       </div>
   </details>

2. <details>
       <summary>Click the <em>Register New Tool</em> button on the right</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/add-tools-step-1.png"
                alt="Click The Register New Tool Button" width="200"/>
       </div>
   </details>

3. <details>
       <summary>Type the Tool repo reference into the modal's input box and click <em>Register Tool</em></summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/add-tools-step-2.png"
                alt="Enter Tool Repo Reference" width="500" height="auto"/>
       </div>
   </details>

<br/>

<details>
    <summary>Once the tool has been added, you can search for it by category or name on the Tools page to verify</summary>
    <div align="left">
        <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/add-tools-step-3.png"
             alt="Search For Newly Added Tools" height="300"/>
    </div>
</details>

## Using The `Hash` Tool in an Agent

To use the `Hash` Tool in an Agent, open the Agent's Edit page, then:

1. <details>
       <summary>Click the <em>Add Tool</em> button under either the <em>Agent Tools</em> or <em>User Tools</em> sections</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/use-tools-step-0.png"
                alt="Click The Add Tool Button" width="500"/>
       </div>
   </details>

2. <details>
       <summary>Search for "Hash" or "Crypto" in the Tool search pop-out and select the <code>Hash</code> Tool</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/use-tools-step-1.png"
                alt="Add Hash Tool To Agent" width="500"/>
       </div>
   </details>

3. <details>
       <summary>Ask the Agent to generate a hash</summary>
       <div align="left">
           <img src="https://raw.githubusercontent.com/otto8-ai/go-hash-tool/refs/heads/main/docs/use-tools-step-2.png"
                alt="Ask The Agent To Generate a Hash" width="500"/>
       </div>
   </details>
