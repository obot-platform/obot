<script lang="ts">
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { ChevronLeft } from 'lucide-svelte';

	interface Props {
		fromURL?: string;
		currentLabel: string;
	}

	let { fromURL, currentLabel }: Props = $props();

	let links = $state<{ href: string; label: string }[]>([]);

	function convertToHistory(href: string) {
		const pathParts = href.split('/').filter(Boolean);
		const [type, id] = pathParts;
		if (type === 'mcp-servers') {
			return [
				{ href: '/v2/admin/mcp-servers', label: 'MCP Servers' },
				...(id ? [convertToMcpLink(id)] : [])
			];
		}

		if (type === 'access-control') {
			return [{ href: '/v2/admin/access-control', label: 'Access Control' }];
		}

		return [];
	}

	$effect(() => {
		if (fromURL) {
			links = [...convertToHistory(fromURL)];
		}
	});

	function convertToMcpLink(id: string) {
		const stringified = sessionStorage.getItem(ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER);
		const json = JSON.parse(stringified ?? '{}');
		const label = id === json.id ? json.name : 'Unknown';
		const href =
			json.type === 'single' || json.type === 'remote'
				? `/v2/admin/mcp-servers/c/${id}`
				: `/v2/admin/mcp-servers/s/${id}`;
		return { href, label };
	}
</script>

<div class="flex flex-wrap items-center">
	{#each links as link, index}
		<ChevronLeft class={index === 0 ? 'mr-2 size-4' : 'mx-2 size-4'} />

		<a href={link.href} class="button-text flex items-center gap-2 p-0 text-lg font-light">
			{link.label}
		</a>
	{/each}
	<ChevronLeft class="mx-2 size-4" />
	<span class="text-lg font-light">{currentLabel}</span>
</div>
