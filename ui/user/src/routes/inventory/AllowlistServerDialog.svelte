<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import {
		ALLOWLIST_SERVER_KIND_LABELS,
		allowlistServerKind,
		allowlistServerProblem,
		PACKAGE_SOURCE_LABELS,
		type AllowlistServerKind
	} from '$lib/enforcement';
	import type { AllowlistServer, AllowlistServerPackageSource } from '$lib/services';
	import { X } from '@lucide/svelte';

	interface Props {
		// index is the position being edited, or undefined when adding.
		onSubmit: (entry: AllowlistServer, index?: number) => void;
	}

	let { onSubmit }: Props = $props();

	const kinds: AllowlistServerKind[] = ['url', 'package', 'hostname', 'connector'];
	const packageSourceOptions = Object.entries(PACKAGE_SOURCE_LABELS).map(([id, label]) => ({
		id,
		label
	}));

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let editingIndex = $state<number>();
	let kind = $state<AllowlistServerKind>('url');
	let url = $state('');
	let hostname = $state('');
	let connector = $state('');
	let packageSource = $state<AllowlistServerPackageSource>('npm');
	let packageName = $state('');
	let packageVersion = $state('');
	let tools = $state<string[]>([]);
	let toolDraft = $state('');

	// The entry as it currently stands, so validation and the submit button react
	// to every keystroke rather than only on submit.
	let entry = $derived.by((): AllowlistServer => {
		const base: AllowlistServer = {};
		switch (kind) {
			case 'url':
				base.url = url;
				break;
			case 'package':
				base.package = { source: packageSource, name: packageName, version: packageVersion };
				break;
			case 'hostname':
				base.hostname = hostname;
				break;
			case 'connector':
				base.connector = connector;
				break;
		}
		if (tools.length > 0) base.tools = tools;
		return base;
	});
	let problem = $derived(allowlistServerProblem(entry));
	// A pristine form has nothing wrong with it yet — the problem only becomes an
	// error worth showing once the administrator has typed something.
	let touched = $derived(
		Boolean(url || hostname || connector || packageName || packageVersion || tools.length > 0)
	);

	export function open(existing?: AllowlistServer, index?: number) {
		editingIndex = index;
		kind = (existing && allowlistServerKind(existing)) || 'url';
		url = existing?.url ?? '';
		hostname = existing?.hostname ?? '';
		connector = existing?.connector ?? '';
		packageSource = existing?.package?.source ?? 'npm';
		packageName = existing?.package?.name ?? '';
		packageVersion = existing?.package?.version ?? '';
		tools = [...(existing?.tools ?? [])];
		toolDraft = '';
		dialog?.open();
	}

	// Switching type clears the other dimensions, so exactly one is ever set and
	// stale input from a type the administrator moved away from can't be submitted.
	function selectKind(next: AllowlistServerKind) {
		if (next === kind) return;
		kind = next;
		url = '';
		hostname = '';
		connector = '';
		packageName = '';
		packageVersion = '';
	}

	function addTool() {
		const tool = toolDraft.trim();
		toolDraft = '';
		if (!tool || tools.includes(tool)) return;
		tools = [...tools, tool];
	}

	function handleToolKeydown(event: KeyboardEvent) {
		if (event.key === 'Enter' || event.key === ',') {
			event.preventDefault();
			addTool();
		} else if (event.key === 'Backspace' && !toolDraft && tools.length > 0) {
			tools = tools.slice(0, -1);
		}
	}

	function handleSubmit() {
		if (problem) return;
		onSubmit(entry, editingIndex);
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={editingIndex === undefined ? 'Allow an MCP Server' : 'Edit Allowed MCP Server'}
	class="w-full max-w-lg"
>
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<span class="input-label">Identify this server by</span>
			<div class="flex flex-wrap gap-2">
				{#each kinds as option (option)}
					<button
						type="button"
						aria-pressed={kind === option}
						class="rounded-full border px-3 py-1 text-sm transition-colors {kind === option
							? 'border-primary bg-primary/10 text-primary'
							: 'border-base-300 dark:border-base-400 text-muted-content hover:text-base-content'}"
						onclick={() => selectKind(option)}
					>
						{ALLOWLIST_SERVER_KIND_LABELS[option]}
					</button>
				{/each}
			</div>
		</div>

		{#if kind === 'url'}
			<div class="flex flex-col gap-1">
				<label for="allowlist-url" class="input-label">Server URL</label>
				<input
					id="allowlist-url"
					type="text"
					bind:value={url}
					placeholder="https://example.com/mcp"
					class="text-input-filled"
				/>
				<span class="input-description">
					Matches on scheme, host, port, and path prefix. Query strings and fragments are not
					allowed.
				</span>
			</div>
		{:else if kind === 'package'}
			<div class="flex flex-col gap-1">
				<span id="allowlist-package-source-label" class="input-label">Registry</span>
				<Select
					id="allowlist-package-source"
					class="bg-base-200 dark:border-base-400 border border-transparent shadow-inner"
					classes={{ root: 'w-40' }}
					options={packageSourceOptions}
					selected={packageSource}
					ariaLabelledby="allowlist-package-source-label"
					onSelect={(option) => (packageSource = option.id as AllowlistServerPackageSource)}
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="allowlist-package-name" class="input-label">Package name</label>
				<input
					id="allowlist-package-name"
					type="text"
					bind:value={packageName}
					placeholder={packageSource === 'npm' ? '@scope/server' : 'mcp-server'}
					class="text-input-filled"
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label for="allowlist-package-version" class="input-label">Version (optional)</label>
				<input
					id="allowlist-package-version"
					type="text"
					bind:value={packageVersion}
					placeholder="Any version"
					class="text-input-filled w-40"
				/>
				<span class="input-description">
					Leave empty to allow any version. Pinning a version re-blocks the server when it is
					upgraded.
				</span>
			</div>
		{:else if kind === 'hostname'}
			<div class="flex flex-col gap-1">
				<label for="allowlist-hostname" class="input-label">Hostname</label>
				<input
					id="allowlist-hostname"
					type="text"
					bind:value={hostname}
					placeholder="gitmcp.io"
					class="text-input-filled"
				/>
				<span class="input-description">
					Allows any MCP server on this hostname, over any path or port.
				</span>
			</div>
		{:else}
			<div class="flex flex-col gap-1">
				<label for="allowlist-connector" class="input-label">Connector name</label>
				<input
					id="allowlist-connector"
					type="text"
					bind:value={connector}
					placeholder="claude.ai Google Calendar"
					class="text-input-filled"
				/>
				<span class="input-description">
					The connector's display name, matched without regard to case. Use this for MCP connectors
					that expose no local URL or command, such as claude.ai Connectors (i.e. claude.ai Google
					Calendar). claude.ai Connectors always have the format "claude.ai &lt;name&gt;".
				</span>
			</div>
		{/if}

		<div class="flex flex-col gap-1">
			<label for="allowlist-tools" class="input-label">Tools</label>
			{#if tools.length > 0}
				<div class="flex flex-wrap gap-1.5 pb-1">
					{#each tools as tool (tool)}
						<span
							class="bg-base-200 dark:bg-base-400 flex items-center gap-1 rounded-full px-2.5 py-1 text-xs"
						>
							{tool}
							<button
								type="button"
								class="text-muted-content hover:text-base-content"
								aria-label={`Remove ${tool}`}
								onclick={() => (tools = tools.filter((candidate) => candidate !== tool))}
							>
								<X class="size-3" />
							</button>
						</span>
					{/each}
				</div>
			{/if}
			<input
				id="allowlist-tools"
				type="text"
				bind:value={toolDraft}
				onkeydown={handleToolKeydown}
				onblur={addTool}
				placeholder="Add a tool name and press Enter"
				class="text-input-filled"
			/>
			<span class="input-description">
				Leave empty to allow every tool on this server. Enforcement never looks at the arguments
				passed to a tool.
			</span>
		</div>

		{#if problem && touched}
			<p class="text-error text-xs">{problem}</p>
		{/if}
	</div>

	<div class="mt-6 flex justify-end gap-2">
		<button class="btn btn-secondary" onclick={() => dialog?.close()}>Cancel</button>
		<button class="btn btn-primary" disabled={Boolean(problem)} onclick={handleSubmit}>
			{editingIndex === undefined ? 'Add Server' : 'Save Server'}
		</button>
	</div>
</ResponsiveDialog>
