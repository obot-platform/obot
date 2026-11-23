---
title: "Claude Code reference deployment"
---

This guide shows how to deploy Claude Code on managed devices so that it uses only the MCP servers you approve. It combines three layers:

1. **Your MDM** installs configuration on the device, keeps it in place, and reports devices that drift.
2. **Claude Code's managed settings** decide which MCP servers Claude Code loads. Claude Code applies them itself, and users can't override them.
3. **Obot Sentry** checks each tool call against your Obot allowlist and records it, alongside local audit logs and device inventory.

Managed settings close the gaps in [Limitations](./enforcement.md#limitations) that come from Claude Code loading MCP configuration that Obot Sentry doesn't read, such as servers passed with `--mcp-config`. Without them, a user can define a server under the same name as an allowed one in a place Obot Sentry doesn't read, and Claude Code runs it while Obot Sentry allows the call.

The setting names and file locations below come from Anthropic's [managed MCP](https://code.claude.com/docs/en/managed-mcp) and [settings](https://code.claude.com/docs/en/settings) documentation. This guide was verified on macOS and Windows with Claude Code 2.1.329 and Obot Sentry 0.1.13. Check Anthropic's pages against the Claude Code version you deploy.

## Requirements {#requirements}

- **Claude Code 2.1.242 or later.** `disableSideloadFlags` and `managedSourcesBehavior` need it. Earlier releases ignore both settings.
- **Obot Sentry 0.1.12 or later**, with [tool call enforcement](./enforcement.md#configure-enforcement) turned on. Earlier releases install Claude Code's hooks in the user's settings file, where some of the settings in this guide turn them off. See [Keep Obot Sentry's hooks in managed settings](#keep-obot-sentrys-hooks-in-managed-settings).

## Choose an approach {#choose-an-approach}

| Approach | What it does | Use it when |
|----------|--------------|-------------|
| [Fixed server list](#fixed-server-list) (`managed-mcp.json`) | Claude Code loads only the servers in this file. Users can't add servers, including plugin servers and servers passed with `--mcp-config`. | Every user can get the same servers, such as a set of Obot vMCPs. |
| [Allowlist](#allowlist) (`allowedMcpServers` in managed settings) | Users can add servers, but Claude Code loads only the ones that match your allowlist and don't match your denylist. | Users need to choose their own servers from an approved set, such as any vMCP on your Obot instance. |

Either way, the [**All Obot-hosted MCP servers**](./enforcement.md#allow-rules) rule in Obot Sentry allows tool calls to any server on your Obot instance, so Obot vMCPs need no other allow rule. The rule matches the scheme, hostname, and port of your Obot server URL. Add rules for any other servers you approve.

## File locations {#file-locations}

Claude Code reads its managed configuration from a machine-wide folder that only administrators can write:

| File | macOS | Windows |
|------|-------|---------|
| `managed-mcp.json` | `/Library/Application Support/ClaudeCode/managed-mcp.json` | `C:\Program Files\ClaudeCode\managed-mcp.json` |
| `managed-settings.json` | `/Library/Application Support/ClaudeCode/managed-settings.json` | `C:\Program Files\ClaudeCode\managed-settings.json` |
| Drop-in settings files | `/Library/Application Support/ClaudeCode/managed-settings.d/*.json` | `C:\Program Files\ClaudeCode\managed-settings.d\*.json` |

- Obot Sentry writes its Claude Code hooks to `managed-settings.json`. Put your own policy in a drop-in file, such as `managed-settings.d/50-obot.json`, so that your MDM and Obot Sentry each own a separate file. Claude Code combines the drop-in files with `managed-settings.json`. See [Keep Obot Sentry's hooks in managed settings](#keep-obot-sentrys-hooks-in-managed-settings).
- Claude Code doesn't read the older `C:\ProgramData\ClaudeCode\` location. Files left there have no effect.
- Keep the files owned by root on macOS, and writable only by SYSTEM and Administrators on Windows. The Obot Sentry MSI creates `C:\Program Files\ClaudeCode` with read-only access for `BUILTIN\Users`.
- Any user on the device can read these files, so don't put secrets in them.
- Claude Code picks up changes to these files in running sessions, without a restart.

:::warning
If a managed settings file isn't a valid JSON object, Claude Code refuses to start. Every launch exits with `Managed settings document could not be parsed as a JSON object; none of its settings are in effect. Fix or remove it.` The same applies to settings delivered as MDM policy. Validate the JSON before you deploy it.
:::

Instead of the settings files, you can deliver the same settings as MDM policy: the `com.anthropic.claudecode` preference domain on macOS, or a `Settings` string value holding the JSON under `HKLM\SOFTWARE\Policies\ClaudeCode` on Windows. MDM policy uses the same keys and behaves the same as the file, but it changes which sources Claude Code applies. Read [Multiple managed sources](#multiple-managed-sources) before you use it.

## Fixed server list {#fixed-server-list}

`managed-mcp.json` uses the same format as a project `.mcp.json` file. List the vMCP connection URLs you copy from Obot:

```json
{
  "mcpServers": {
    "obot-engineering": {
      "type": "http",
      "url": "https://obot.example.com/mcp-connect/<vmcp-id>"
    },
    "obot-support": {
      "type": "http",
      "url": "https://obot.example.com/mcp-connect/<vmcp-id>"
    }
  }
}
```

When this file exists:

- Claude Code loads no other MCP servers. It ignores servers from user and project configuration, installed plugins, and `--plugin-dir`. `claude mcp add` fails with `enterprise MCP configuration is active and has exclusive control over MCP servers`.
- Starting Claude Code with `--mcp-config` or `--strict-mcp-config` fails with an error.
- claude.ai connectors are turned off. To turn them back on, set `"allowAllClaudeAiMcps": true` in managed settings. The same key in a user's own settings has no effect.
- Obot Sentry reads this file first and blocks any server that isn't in it. If a user's configuration reuses a managed server's name for a different server, Claude Code and Obot Sentry both use the managed entry.

Because every server in this example is a remote Obot vMCP, there is no local package runner for a user to replace. Users sign in to Obot the first time they connect, and Obot's access controls decide which tools each user gets.

## Allowlist {#allowlist}

Set the allowlist in a managed settings drop-in file, such as `managed-settings.d/50-obot.json`:

```json
{
  "allowManagedMcpServersOnly": true,
  "allowedMcpServers": [
    { "serverUrl": "https://obot.example.com/*" }
  ],
  "disableSideloadFlags": true,
  "allowManagedHooksOnly": true
}
```

- `allowedMcpServers` lists the servers Claude Code may load. Each entry has one key: `serverUrl` (wildcards allowed), `serverCommand` (an exact command and argument list), or `serverName`. A `serverName` entry matches only the name a user gives the server, so it isn't a security control. Use `serverUrl` or `serverCommand`. An empty list allows no servers.
- `deniedMcpServers`, if you add it, blocks servers from any source. A denylist match always wins over the allowlist.
- `allowManagedMcpServersOnly` makes the managed allowlist the only one Claude Code uses. Without it, Claude Code also honors `allowedMcpServers` entries in the user's own settings.
- `disableSideloadFlags` makes Claude Code reject the `--mcp-config`, `--plugin-dir`, `--plugin-url`, and `--agents` flags, and the `CLAUDE_CODE_PLUGIN_DIRS` environment variable, at startup. It requires Claude Code 2.1.242 or later. It doesn't reject `--strict-mcp-config`, but the allowlist still applies in that mode.
- `allowManagedHooksOnly` makes Claude Code ignore hooks the user adds. See [Keep Obot Sentry's hooks in managed settings](#keep-obot-sentrys-hooks-in-managed-settings).

Claude Code applies the allowlist to servers from every source: user and project configuration, installed plugins, `--plugin-dir`, `--mcp-config`, and claude.ai connectors. When it filters a server out, it prints a warning such as `MCP server blocked by enterprise policy`.

A `serverCommand` entry must match the command exactly as the user configures it, including how the path is spelled. On Windows, `C:/tools/server.exe` doesn't match a server configured as `C:\tools\server.exe`. A `serverUrl` allowlist for your Obot instance isn't affected.

### Shadowed server names {#shadowed-server-names}

With the allowlist in place, a server that Claude Code filters out can't take over an allowed server's name:

- If `--mcp-config` redefines an allowed server's name as a server that isn't allowed, Claude Code drops that definition and uses the allowed one. Obot Sentry resolves the same definition.
- If a project's `.mcp.json` redefines the name of an allowed user-level server, Claude Code drops the name, so neither server can be called.

Without the allowlist, the same `--mcp-config` redefinition runs the user's server while Obot Sentry checks and allows the Obot vMCP. Use the allowlist or a fixed server list on every device where you turn on enforcement.

## Keep Obot Sentry's hooks in managed settings {#keep-obot-sentrys-hooks-in-managed-settings}

Obot Sentry 0.1.12 and later installs its Claude Code hooks in `managed-settings.json`. Because they're managed hooks, they keep running under the settings that turn off user hooks:

- `allowManagedHooksOnly: true`
- `strictPluginOnlyCustomization: true`
- `strictPluginOnlyCustomization` set to a list that includes `"hooks"`

Set `allowManagedHooksOnly` so that a user's own hooks can't change a tool call before Obot Sentry checks it. `strictPluginOnlyCustomization: true` also locks MCP configuration, which stops servers in `~/.claude.json` and `.mcp.json` from loading, and it turns off plugins that aren't from a managed marketplace.

:::warning
Obot Sentry 0.1.11 and earlier install the hooks in the user's `~/.claude/settings.json`. On those releases, each of the settings above turns off enforcement and auditing for Claude Code. Upgrade Obot Sentry before you set them.
:::

Obot Sentry merges its hooks into an existing `managed-settings.json` and keeps the other keys in it. If your MDM later replaces the whole file, the hooks are gone until Obot Sentry's next hourly hook installation puts them back, and Claude Code calls aren't checked in the meantime. To avoid that gap, deliver your own policy in a `managed-settings.d/` drop-in file and leave `managed-settings.json` to Obot Sentry.

## Multiple managed sources {#multiple-managed-sources}

Claude Code can receive managed settings from several sources. From highest rank to lowest:

1. Server-managed settings from claude.ai. See [Server-managed settings](#server-managed-settings).
2. MDM policy: the `com.anthropic.claudecode` profile on macOS, or `HKLM\SOFTWARE\Policies\ClaudeCode` on Windows.
3. The managed settings file, `managed-settings.json`, together with its `managed-settings.d/` drop-in files.
4. On Windows, `HKCU\SOFTWARE\Policies\ClaudeCode`. Users can write this key, so Claude Code reads it only when no managed settings file exists. Obot Sentry creates the file, so a user can't use this key to override it.

By default (`managedSourcesBehavior` set to `first-wins`), Claude Code applies only the highest-ranked source it finds and skips the rest. A few MCP keys, such as `allowManagedMcpServersOnly`, `deniedMcpServers`, and `allowAllClaudeAiMcps`, still apply from every source, but `hooks` doesn't. So any MDM policy for Claude Code, even one with a single unrelated key, makes Claude Code skip the managed settings file, including Obot Sentry's hooks and your drop-in files. Claude Code calls are then neither checked nor audited. `managed-mcp.json` isn't affected.

To apply every source, set `managedSourcesBehavior` to `merge` (Claude Code 2.1.242 or later):

```json
{
  "managedSourcesBehavior": "merge"
}
```

Set it in every device-delivered source that outranks the file: the `com.anthropic.claudecode` profile on macOS and the `HKLM` policy value on Windows. Set it there even if your server-managed settings already set it. A user can make Claude Code skip server-managed settings, and their `merge` setting with them, for example by setting `ANTHROPIC_BASE_URL` in their shell or in the `env` block of `~/.claude/settings.json`. The device-delivered source then becomes the highest-ranked one.

If you deliver no Claude Code MDM policy, you don't need to set anything on the device. Server-managed settings still need `merge`, as described in [Server-managed settings](#server-managed-settings).

## Deliver and monitor with your MDM {#deliver-and-monitor-with-your-mdm}

1. Deploy Obot Sentry 0.1.12 or later with enforcement turned on, by following the `INSTRUCTIONS.md` in your device configuration download. See [Enroll devices](./enroll.md).
2. Deploy `managed-mcp.json`, a `managed-settings.d/` drop-in file with your allowlist, or both, to the [file locations](#file-locations) above, writable only by administrators. Validate the JSON before you deploy it.
3. If you also deliver Claude Code settings as MDM policy, set `managedSourcesBehavior` to `merge` in it. See [Multiple managed sources](#multiple-managed-sources).
4. Add a [compliance check](#compliance-check) that reports devices where Obot Sentry's hook or your MCP restriction is missing.
5. [Verify the deployment](#verify-the-deployment) on a test device.

### Compliance check {#compliance-check}

The scripts below report a device as out of compliance when:

- Obot Sentry isn't installed, or (on Windows) enforcement isn't turned on.
- Obot Sentry's Claude Code hook is missing from `managed-settings.json`.
- Neither `managed-mcp.json` nor `allowManagedMcpServersOnly` is in place.
- A Claude Code MDM policy doesn't set `managedSourcesBehavior` to `merge`.
- A policy file can be written by non-administrators, or (on Windows) the `HKLM` policy value isn't valid JSON.

Adjust the paths and checks to match your deployment.

<details>
<summary>macOS (Jamf Extension Attribute or Intune custom attribute)</summary>

Add this as a Jamf Extension Attribute with the string data type, or as an Intune macOS custom attribute. It prints `Compliant`, or `Non-compliant:` followed by the problems it found.

```bash
#!/bin/bash
# Claude Code + Obot Sentry compliance check for macOS.
# Jamf: Extension Attribute (string). Intune: macOS shell script / custom attribute.
# Prints one <result>…</result> line; "Compliant" or "Non-compliant: <reasons>".
DIR="/Library/Application Support/ClaudeCode"
SETTINGS="$DIR/managed-settings.json"
PLIST="/Library/Managed Preferences/com.anthropic.claudecode.plist"
problems=()

get() { /usr/bin/plutil -extract "$2" raw -o - "$1" 2>/dev/null; }

# Sentry's enforcement hook (obot-sentry v0.1.12+ writes it to managed-settings.json).
[[ -x /usr/local/bin/obot-sentry ]] || problems+=("obot-sentry missing")
/usr/bin/grep -qs 'obot-sentry enforce --agent claude-code' "$SETTINGS" \
  || problems+=("Sentry PreToolUse hook missing from managed-settings.json")

# An MCP restriction: managed-mcp.json, or allowManagedMcpServersOnly in a file, drop-in, or profile.
locked=""
[[ -f "$DIR/managed-mcp.json" ]] && locked=1
for src in "$SETTINGS" "$DIR"/managed-settings.d/*.json "$PLIST"; do
  [[ -f "$src" && "$(get "$src" allowManagedMcpServersOnly)" == "true" ]] && locked=1
done
[[ -n "$locked" ]] || problems+=("no managed-mcp.json or allowManagedMcpServersOnly")

# A Claude Code MDM profile outranks the file; under the default first-wins it hides Sentry's hooks.
if [[ -f "$PLIST" && "$(get "$PLIST" managedSourcesBehavior)" != "merge" ]]; then
  problems+=("com.anthropic.claudecode profile without managedSourcesBehavior=merge")
fi

# Policy files must be root-owned and not writable by others.
for f in "$SETTINGS" "$DIR/managed-mcp.json" "$DIR"/managed-settings.d/*.json; do
  [[ -f "$f" ]] || continue
  [[ "$(/usr/bin/stat -f %Su "$f")" == root ]] || problems+=("$(basename "$f") not owned by root")
  [[ "$(/usr/bin/stat -f %Lp "$f")" =~ [2367]$ ]] && problems+=("$(basename "$f") world-writable")
done

if (( ${#problems[@]} )); then
  (IFS=';'; echo "<result>Non-compliant: ${problems[*]}</result>")
else
  echo "<result>Compliant</result>"
fi
```

</details>

<details>
<summary>Windows (Intune)</summary>

Use this as an Intune Remediations detection script, or as a custom compliance discovery script. Run it as SYSTEM in 64-bit PowerShell. It prints one JSON object with `ClaudeCodeCompliant` and `Problems`, and exits with 0 when the device is compliant and 1 when it isn't.

```powershell
# Claude Code + Obot Sentry compliance check for Windows.
# Intune: use as a Remediations detection script, or as a custom compliance
# discovery script (it prints one compressed JSON object). Run it as SYSTEM in
# 64-bit PowerShell. Exit code 0 = compliant, 1 = non-compliant.
$ErrorActionPreference = 'Stop'
$dir      = 'C:\Program Files\ClaudeCode'
$settings = Join-Path $dir 'managed-settings.json'
$mcp      = Join-Path $dir 'managed-mcp.json'
$dropins  = @(Get-ChildItem (Join-Path $dir 'managed-settings.d') -Filter *.json -File -ErrorAction SilentlyContinue)
$sentry   = 'C:\Program Files\Obot\obot-sentry\obot-sentry.exe'
$problems = New-Object System.Collections.Generic.List[string]

function Read-Json([string]$path) {
    try { return Get-Content -Raw -LiteralPath $path | ConvertFrom-Json } catch { $problems.Add("$(Split-Path -Leaf $path) is not valid JSON"); return $null }
}

# Sentry is installed, enforcing, and its PreToolUse hook is in managed-settings.json.
if (-not (Test-Path -LiteralPath $sentry)) { $problems.Add('obot-sentry.exe missing') }
$sentryCfg = Get-ItemProperty 'HKLM:\SOFTWARE\Obot\obot-sentry' -ErrorAction SilentlyContinue
if ($sentryCfg.EnforcementEnabled -ne 1) { $problems.Add('Sentry EnforcementEnabled is not 1') }
$main = if (Test-Path -LiteralPath $settings) { Read-Json $settings } else { $null }
$hook = $main.hooks.PreToolUse | ForEach-Object { $_.hooks } |
    Where-Object { $_.command -like '*obot-sentry.exe*enforce --agent claude-code*' }
if (-not $hook) { $problems.Add('Sentry PreToolUse hook missing from managed-settings.json') }

# An MCP restriction: managed-mcp.json, or allowManagedMcpServersOnly in the file, a drop-in, or HKLM policy.
$hklmJson = (Get-ItemProperty 'HKLM:\SOFTWARE\Policies\ClaudeCode' -ErrorAction SilentlyContinue).Settings
$hklm = $null
if ($hklmJson) { try { $hklm = $hklmJson | ConvertFrom-Json } catch { $problems.Add('HKLM ClaudeCode Settings is not valid JSON (Claude Code will not start)') } }
$sources = @($main, $hklm) + @($dropins | ForEach-Object { Read-Json $_.FullName })
$locked = (Test-Path -LiteralPath $mcp) -or [bool]($sources | Where-Object { $_.allowManagedMcpServersOnly -eq $true })
if (-not $locked) { $problems.Add('no managed-mcp.json or allowManagedMcpServersOnly') }

# HKLM policy outranks the file. Under the default first-wins it hides Sentry's hooks unless it sets merge.
if ($hklm) {
    $policyKeys = @($hklm.PSObject.Properties.Name | Where-Object { $_ -notin 'managedSourcesBehavior', 'wslInheritsWindowsSettings' })
    if ($policyKeys.Count -gt 0 -and $hklm.managedSourcesBehavior -ne 'merge') {
        $problems.Add('HKLM\SOFTWARE\Policies\ClaudeCode without managedSourcesBehavior=merge')
    }
}

# Policy files must not be writable by non-admins.
$userSids = 'S-1-1-0', 'S-1-5-11', 'S-1-5-32-545', 'S-1-5-4'   # Everyone, Authenticated Users, Users, INTERACTIVE
# WriteData/CreateFiles, AppendData/CreateDirectories, WriteExtendedAttributes, DeleteSubdirectoriesAndFiles,
# WriteAttributes, Delete, ChangePermissions, TakeOwnership, GENERIC_ALL, GENERIC_WRITE
$write = 0x2 -bor 0x4 -bor 0x10 -bor 0x40 -bor 0x100 -bor 0x10000 -bor 0x40000 -bor 0x80000 -bor 0x10000000 -bor 0x40000000
foreach ($f in @($dir, $settings, $mcp) + @($dropins | ForEach-Object FullName)) {
    if (-not (Test-Path -LiteralPath $f)) { continue }
    $acl = Get-Acl -LiteralPath $f
    foreach ($r in $acl.Access) {
        $sid = try { $r.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value } catch { '' }
        if ($r.AccessControlType -eq 'Allow' -and $sid -in $userSids -and ($r.FileSystemRights -band $write)) {
            $problems.Add("$(Split-Path -Leaf $f) writable by $($r.IdentityReference)"); break
        }
    }
}

$result = [ordered]@{ ClaudeCodeCompliant = ($problems.Count -eq 0); Problems = ($problems -join '; ') }
$result | ConvertTo-Json -Compress
if ($problems.Count) { exit 1 } else { exit 0 }
```

</details>

### Verify the deployment {#verify-the-deployment}

On a test device:

1. Run `/status` in Claude Code. Check that it lists **Enterprise managed settings** and that the sources in parentheses are the ones you deploy. For example, `(file)` means only the managed settings file applies, and `(remote + HKLM + file, merged)` means server-managed settings, the Windows policy, and the file all apply. `drop-ins` appears when a `managed-settings.d/` file applies. If `/status` shows a `Skipped sources:` line that includes the file, Obot Sentry's hooks aren't running; see [Multiple managed sources](#multiple-managed-sources). `claude doctor` doesn't show this information.
2. Run `claude mcp list` and check that only the servers you allow are listed.
3. Make a tool call to an Obot vMCP and confirm that it's allowed and appears in Obot's [enforcement decisions](./enforcement.md#review-enforcement-decisions).
4. Start Claude Code with `--mcp-config` and confirm that it's rejected, or that the server it adds is blocked by enterprise policy.

## Server-managed settings {#server-managed-settings}

On a Claude for Teams or Claude for Enterprise plan, an organization Owner can also deliver Claude Code settings from **Organization settings > Claude Code > Managed settings** in Claude. See Anthropic's [server-managed settings documentation](https://code.claude.com/docs/en/server-managed-settings).

Server-managed settings rank above every other source, so if you use them, set `managedSourcesBehavior` to `merge` in them. Otherwise Claude Code skips the managed settings file on every device, including Obot Sentry's hooks.

Server-managed settings add to the MDM-delivered files, but they can't replace them. Claude Code applies them only to sessions signed in to your organization's Claude account, and a user can skip them: for example, by setting `ANTHROPIC_BASE_URL`, using Amazon Bedrock or Google Vertex AI, or signing in with an API key from a different Console organization. Deliver your MCP restrictions with your MDM on managed devices, and set `merge` in your device-delivered MDM policy as described in [Multiple managed sources](#multiple-managed-sources).
