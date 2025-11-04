<script lang="ts">
	import { onNavigate } from '$app/navigation';
	import { ADMIN_SESSION_STORAGE } from '$lib/constants';
	import { getSearchParamsFromLocalStorage } from '$lib/url';
	import { openUrl } from '$lib/utils';
	import { ChevronLeft } from 'lucide-svelte';

	interface Props {
		fromURL?: string;
		currentLabel: string;
	}

	let { fromURL, currentLabel }: Props = $props();

	let links = $state<{ href: string; label: string }[]>([]);

	function convertToHistory(href: string) {
		const pathParts = href.split('/').filter(Boolean);
		// Find the admin section part (skip admin/)
		const adminIndex = pathParts.findIndex((part) => part === 'admin');
		const path = adminIndex >= 0 ? pathParts.slice(adminIndex + 1) : pathParts;
		const [type, id] = path;
		if (type === 'mcp-servers') {
			return [
				{ href: '/admin/mcp-servers', label: 'MCP Servers' },
				...(id ? [convertToMcpLink(id, true)] : [])
			];
		}

		if (type === 'access-control') {
			return [{ href: '/admin/access-control', label: 'Access Control' }];
		}

		if (type === 'filters') {
			return [{ href: '/admin/filters', label: 'Filters' }];
		}

		if (type === 'chat-threads') {
			return [{ href: '/admin/chat-threads', label: 'Chat Threads' }];
		}

		if (type === 'tasks') {
			return [{ href: '/admin/tasks', label: 'Tasks' }];
		}

		if (type === 'task-runs') {
			return [{ href: '/admin/task-runs', label: 'Task Runs' }];
		}

		if (type === 'mcp-publisher') {
			if (id === 'access-control') {
				return [{ href: `/mcp-publisher/access-control`, label: 'Access Control' }];
			}
			return [
				{
					href: '/mcp-publisher',
					label: 'MCP Servers'
				},
				...(id ? [convertToMcpLink(id, false)] : [])
			];
		}

		if (type === 'deployed-servers') {
			return [{ href: '/admin/mcp-servers', label: 'MCP Servers' }];
		}

		if (type === 'audit-logs-exports') {
			return [{ href: '/admin/audit-logs/exports', label: 'Audit Log Exports' }];
		}

		if (type === 'audit-logs') {
			return [{ href: '/admin/audit-logs', label: 'Audit Logs' }];
		}

		return [];
	}

	$effect(() => {
		if (fromURL) {
			links = [...convertToHistory(fromURL)];
		}
	});

	onNavigate(() => {
		if (fromURL) {
			links = [...convertToHistory(fromURL)];
		}
	});

	function convertToMcpLink(id: string, isAdmin: boolean) {
		const stringified = sessionStorage.getItem(ADMIN_SESSION_STORAGE.LAST_VISITED_MCP_SERVER);
		const json = JSON.parse(stringified ?? '{}');
		const label = id === json.id ? json.name : 'Unknown';

		if (json.serverId) {
			let href = '';
			if (isAdmin) {
				href =
					json.type !== 'multi'
						? `/admin/mcp-servers/c/${id}/instance/${json.serverId}`
						: `/admin/mcp-servers/s/${id}/details`;
			} else {
				href =
					json.type !== 'multi'
						? `/mcp-publisher/c/${id}/instance/${json.serverId}`
						: `/mcp-publisher/s/${id}/details`;
			}

			if (json.prevFrom) {
				href = `${href}?from=${json.prevFrom}`;
			}
			console.log({ href, label });
			return { href, label };
		}

		if (json.entity === 'workspace') {
			let href = '';
			if (isAdmin) {
				href =
					json.type !== 'multi'
						? `/admin/mcp-servers/w/${json.entityId}/c/${id}`
						: `/admin/mcp-servers/w/${json.entityId}/s/${id}`;
			} else {
				href = json.type !== 'multi' ? `/mcp-publisher/c/${id}` : `/mcp-publisher/s/${id}`;
			}
			return { href, label };
		}

		const href =
			json.type !== 'multi' ? `/admin/mcp-servers/c/${id}` : `/admin/mcp-servers/s/${id}`;
		return { href, label };
	}
</script>

<div class="flex flex-wrap items-center capitalize">
	{#each links as link, index (link.href)}
		<ChevronLeft class={index === 0 ? 'mr-2 size-4' : 'mx-2 size-4'} />

		<button
			onclick={(e) => {
				const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;
				const isCtrlClick = isTouchDevice ? false : e.metaKey || e.ctrlKey;

				const searchParams = getSearchParamsFromLocalStorage(link.href);
				const url = [link.href, searchParams].filter(Boolean).join('');

				openUrl(url, isCtrlClick);
			}}
			class="button-text flex items-center gap-2 p-0 text-lg font-light"
		>
			{link.label}
		</button>
	{/each}
	<ChevronLeft class="mx-2 size-4" />
	<span class="text-lg font-light">{currentLabel}</span>
</div>
