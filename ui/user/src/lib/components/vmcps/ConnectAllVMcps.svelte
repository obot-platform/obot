<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import type { VMCP } from '$lib/services';
	import { AiClient, COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import { buildConnectAllSnippets, vmcpConnectURL } from '$lib/services/vmcps/utils';
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
	let hasConnectableVMcps = $derived(
		vmcps.some((vmcp) => (vmcp.components?.length ?? 0) > 0 && !!vmcpConnectURL(vmcp))
	);
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
			{m.vmcps_connect_all_vmcps()}
		{/if}
	{/snippet}
	<div class="flex flex-col gap-3 md:p-0 p-4">
		{#if !hasConnectableVMcps}
			<p class="text-sm text-muted-content font-light">
				{m.vmcps_connect_all_none()}
			</p>
		{:else if selectedConnectAllSnippet}
			{#if connectAllSnippets.length > 1}
				<div role="tablist" class="tabs tabs-box" aria-label={m.vmcps_configuration_files()}>
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
									{m.vmcps_connect_all_claude_admin_prefix()}<code class="text-base-content"
										>Admin Settings > Claude Code > Managed settings</code
									>{m.vmcps_connect_all_claude_admin_suffix()}
								</p>
							{:else}
								<p>
									{m.vmcps_connect_all_claude_prefix()}<code class="text-base-content"
										>.mcp.json</code
									>{m.vmcps_connect_all_claude_middle()}<code class="text-base-content"
										>~/.claude.json</code
									>{m.vmcps_connect_all_claude_suffix()}
								</p>
							{/if}
						{:else if selectedClient.id === AiClient.Codex}
							<p>
								{m.vmcps_connect_all_codex_prefix()}<code class="text-base-content"
									>~/.codex/config.toml</code
								>{m.vmcps_connect_all_codex_middle()}<code class="text-base-content"
									>.codex/config.toml</code
								>{m.vmcps_connect_all_codex_suffix()}
							</p>
						{:else if selectedClient.id === AiClient.Cursor}
							<p>
								{m.vmcps_connect_all_cursor_prefix()}<code class="text-base-content"
									>~/.cursor/mcp.json</code
								>{m.vmcps_connect_all_cursor_middle()}<code class="text-base-content"
									>.cursor/mcp.json</code
								>{m.vmcps_connect_all_cursor_suffix()}
							</p>
						{:else if selectedClient.id === AiClient.VSCode}
							<p>
								{m.vmcps_connect_all_vscode_prefix()}<code class="text-base-content"
									>.vscode/mcp.json</code
								>{m.vmcps_connect_all_vscode_suffix()}
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
