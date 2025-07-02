<script lang="ts">
	import { getAdminMcpServerAndEntries } from '$lib/context/admin/mcpServerAndEntries.svelte';
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
				...(id ? [{ href: `/v2/admin/mcp-servers?id=${id}`, label: convertToLabel(href) }] : [])
			];
		}

		if (type === 'access-control') {
			return [{ href: '/v2/admin/access-control', label: 'Access Control' }];
		}

		return [];
	}

	$effect(() => {
		if (
			fromURL &&
			(mcpServerAndEntries.servers.length > 0 || mcpServerAndEntries.entries.length > 0)
		) {
			links = [...convertToHistory(fromURL)];
		}
	});

	const mcpServerAndEntries = getAdminMcpServerAndEntries();

	function convertToLabel(href: string) {
		const pathParts = href.split('/').filter(Boolean);
		const [type, id] = pathParts;
		let label: string | undefined = undefined;
		if (type === 'mcp-servers') {
			const match =
				mcpServerAndEntries.entries.find((e) => e.id === id) ||
				mcpServerAndEntries.servers.find((s) => s.id === id);
			if (match) {
				if ('manifest' in match) {
					label = match.manifest.name;
				} else {
					label = match.commandManifest?.name || match.urlManifest?.name || 'Unknown';
				}
			}
		}

		return label || 'Unknown';
	}
</script>

<div class="flex flex-wrap items-center">
	{#each links as link}
		<ChevronLeft class="mx-2 size-4" />

		<a href={link.href} class="button-text flex items-center gap-2 p-0 text-lg font-light">
			{link.label}
		</a>
	{/each}
	<ChevronLeft class="mx-2 size-4" />
	<span class="text-lg font-light">{currentLabel}</span>
</div>
