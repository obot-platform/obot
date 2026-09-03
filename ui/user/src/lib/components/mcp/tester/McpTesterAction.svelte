<script lang="ts">
	import { resolve } from '$app/paths';
	import type { MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
	import { canTestMCPServer, getMCPDisplayName } from '$lib/services/user/mcp';
	import { mcpServersAndEntries } from '$lib/stores';
	import { FlaskConical } from '@lucide/svelte';

	interface Props {
		server?: MCPCatalogServer;
		entry?: MCPCatalogEntry;
		forceVisible?: boolean;
		disabled?: boolean;
		ontest?: () => void;
	}

	let { server, entry, forceVisible = false, disabled = false, ontest }: Props = $props();
	let authorizedServers = $derived([
		...mcpServersAndEntries.current.servers,
		...mcpServersAndEntries.current.userConfiguredServers
	]);
	let visible = $derived(forceVisible || canTestMCPServer(server, authorizedServers));
	let serverName = $derived(getMCPDisplayName(server ?? entry, 'MCP Server'));
</script>

{#if visible && (server || entry)}
	{#if disabled || ontest || !server}
		<button
			type="button"
			class="btn btn-secondary flex w-full items-center gap-1 text-sm md:w-fit"
			{disabled}
			aria-label={`Test ${serverName}`}
			onclick={(event) => {
				event.stopPropagation();
				ontest?.();
			}}
		>
			<FlaskConical class="size-4" aria-hidden="true" />
			Test
		</button>
	{:else}
		<a
			class="btn btn-secondary flex w-full items-center gap-1 text-sm md:w-fit"
			href={resolve(`/mcp-servers/test/${encodeURIComponent(server.id)}`)}
			aria-label={`Test ${serverName}`}
			onclick={(event) => event.stopPropagation()}
		>
			<FlaskConical class="size-4" aria-hidden="true" />
			Test
		</a>
	{/if}
{/if}
