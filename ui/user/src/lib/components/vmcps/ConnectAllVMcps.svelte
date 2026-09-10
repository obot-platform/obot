<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { VMCP } from '$lib/services';
	import { AiClient, COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import { buildConnectAllSnippets } from '$lib/services/vmcps/utils';
	import { profile } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcps: VMCP[];
	}

	let { vmcps }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedClient = $state<(typeof COMMON_AI_CLIENTS)[number]>();
	let selectedConnectAllSnippetId = $state<string>();
	let isAdmin = $derived(!!profile.current.isAdmin?.());
	let connectAllSnippets = $derived(
		selectedClient ? buildConnectAllSnippets(selectedClient.id, vmcps, isAdmin) : []
	);
	let selectedConnectAllSnippet = $derived(
		connectAllSnippets.find((snippet) => snippet.id === selectedConnectAllSnippetId) ??
			connectAllSnippets[0]
	);

	export function open(client: (typeof COMMON_AI_CLIENTS)[number]) {
		selectedClient = client;
		selectedConnectAllSnippetId = undefined;
		dialog?.open();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	id="connect-all-vmcps-dialog"
	onClose={() => {
		selectedClient = undefined;
		selectedConnectAllSnippetId = undefined;
	}}
>
	{#snippet titleContent()}
		{#if selectedClient}
			<img src={selectedClient.icon} alt="" class="mt-0.5 size-4 block dark:hidden" />
			<img
				src={selectedClient.iconDark ?? selectedClient.icon}
				alt=""
				class="mt-0.5 size-4 hidden dark:block"
			/>
			Connect All vMCPs
		{/if}
	{/snippet}
	<div class="flex flex-col gap-3 md:p-0 p-4">
		{#if vmcps.length === 0}
			<p class="text-sm text-muted-content font-light">
				No vMCPs currently have a connection URL to copy.
			</p>
		{:else if selectedConnectAllSnippet}
			{#if connectAllSnippets.length > 1}
				<div role="tablist" class="tabs tabs-box" aria-label="Configuration files">
					{#each connectAllSnippets as snippet (snippet.id)}
						<button
							type="button"
							role="tab"
							aria-selected={selectedConnectAllSnippet.id === snippet.id}
							aria-controls="connect-all-snippet-panel"
							class={twMerge('tab', selectedConnectAllSnippet.id === snippet.id && 'tab-active')}
							onclick={() => (selectedConnectAllSnippetId = snippet.id)}
						>
							{snippet.label}
						</button>
					{/each}
				</div>
			{/if}
			{#if selectedClient}
				<div class="flex items-start gap-2 text-sm">
					<div class="flex flex-col gap-2 text-muted-content font-light">
						{#if selectedClient.id === AiClient.Claude}
							{#if isAdmin && selectedConnectAllSnippet.id === 'claude-settings-json'}
								<p>
									Go to <code class="text-base-content"
										>Admin Settings > Claude Code > Managed settings</code
									> and add the following configuration JSON:
								</p>
							{:else}
								<p>
									Copy the configuration below into your project's <code class="text-base-content"
										>.mcp.json</code
									>
									or your user-level
									<code class="text-base-content">~/.claude.json</code>.
								</p>
							{/if}
						{:else if selectedClient.id === AiClient.Codex}
							<p>
								Copy these tables into
								<code class="text-base-content">~/.codex/config.toml</code>
								or a project-scoped
								<code class="text-base-content">.codex/config.toml</code>.
							</p>
						{:else if selectedClient.id === AiClient.Cursor}
							<p>
								Copy the configuration below into
								<code class="text-base-content">~/.cursor/mcp.json</code>
								or your project's
								<code class="text-base-content">.cursor/mcp.json</code>.
							</p>
						{:else if selectedClient.id === AiClient.VSCode}
							<p>
								Copy this configuration into your workspace
								<code class="text-base-content">.vscode/mcp.json</code>.
							</p>
						{/if}
					</div>
				</div>
			{/if}
			<div class="relative" id="connect-all-snippet-panel" role="tabpanel">
				<pre
					class="pl-4 pr-22 py-2 m-0 max-h-96 overflow-y-auto dark:bg-base-200"
					id={`connect-all-mcp-json-${selectedConnectAllSnippet.id}`}><code
						class="font-mono text-xs">{selectedConnectAllSnippet.value}</code
					></pre>
				<div class="absolute top-4 right-4">
					<CopyButton
						text={selectedConnectAllSnippet.value}
						id={`connect-all-mcp-json-copy-button-${selectedConnectAllSnippet.id}`}
						classes={{ button: 'flex shrink-0 gap-2 text-xs' }}
						showTextLeft
					/>
				</div>
			</div>
		{/if}
	</div>
</ResponsiveDialog>
