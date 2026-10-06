<script lang="ts">
	import { m } from '$lib/i18n';
	import type { MCPCatalogServer } from '$lib/services';
	import { getMCPDisplayName } from '$lib/services/user/mcp';
	import ResponsiveDialog from '../../ResponsiveDialog.svelte';
	import DebugOauthFlow from './DebugOauthFlow.svelte';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let serverToDebug = $state<MCPCatalogServer>();

	export function open(server: MCPCatalogServer) {
		serverToDebug = server;
		dialog?.open();
	}

	function close() {
		serverToDebug = undefined;
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="md:max-w-4xl"
	classes={{ content: 'p-0', header: 'md:p-4 md:pb-0' }}
	onClose={close}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			<div class="p-0.5 rounded-sm dark:bg-base-400">
				<img
					src={serverToDebug?.manifest.icon ?? ''}
					alt={serverToDebug?.manifest.name ?? ''}
					class="size-6"
				/>
			</div>
			{m.mcps_oauth_debug_oauth_dialog_title({
				name: getMCPDisplayName(serverToDebug, m.mcps_server_fallback_name())
			})}
		</div>
	{/snippet}
	{#if serverToDebug}
		<DebugOauthFlow mcpServer={serverToDebug} classes={{ footer: 'sticky bottom-0 left-0' }} />
	{/if}
</ResponsiveDialog>
