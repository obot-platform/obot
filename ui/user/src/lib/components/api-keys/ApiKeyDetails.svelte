<script lang="ts">
	import type { APIKey } from '$lib/services/api-keys/types';
	import { stripMarkdownToText } from '$lib/markdown';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import { Server, Trash2 } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { fly } from 'svelte/transition';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { compileAvailableMcpServers } from '$lib/services/chat/mcp';
	import Confirm from '../Confirm.svelte';

	interface Props {
		apiKey?: APIKey & { prefix: string };
		onDelete: () => void;
	}

	let { apiKey, onDelete }: Props = $props();
	let deletingApiKey = $state(false);
	let saving = $state(false);

	let mcpServers = $derived(
		compileAvailableMcpServers(
			mcpServersAndEntries.current.servers,
			mcpServersAndEntries.current.userConfiguredServers
		)
	);

	let serverMap = $derived(new Map(mcpServers.map((s) => [s.id, s])));

	let isAllServers = $derived(apiKey?.mcpServerIds?.includes('*') ?? false);

	let resolvedServers = $derived.by(() => {
		if (!apiKey?.mcpServerIds || isAllServers) return [];
		return apiKey.mcpServerIds.map((id) => {
			const server = serverMap.get(id);
			return {
				id,
				name: server?.alias || server?.manifest.name || '(Deleted)',
				description: server?.manifest.description,
				icon: server?.manifest.icon,
				exists: !!server
			};
		});
	});

	let createdDisplay = $derived(apiKey ? formatTimeAgo(apiKey.createdAt).relativeTime : '');
	let lastUsedDisplay = $derived(
		apiKey?.lastUsedAt ? formatTimeAgo(apiKey.lastUsedAt).relativeTime : 'Never'
	);
	let expiresDisplay = $derived(
		apiKey?.expiresAt ? formatTimeUntil(apiKey.expiresAt).relativeTime : 'Never'
	);

	const duration = PAGE_TRANSITION_DURATION;
</script>

{#if apiKey}
	<div
		class="flex h-full w-full flex-col gap-4"
		out:fly={{ x: 100, duration }}
		in:fly={{ x: 100, delay: duration }}
	>
		<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
			<div class="flex w-full items-center justify-between gap-4">
				<h1 class="flex items-center gap-4 text-2xl font-semibold">
					{apiKey.name || 'API Key'}
				</h1>
				{#if apiKey.userId.toString() === profile.current.id}
					<button
						class="button-destructive flex items-center gap-1 text-xs font-normal"
						use:tooltip={'Delete API Key'}
						disabled={saving}
						onclick={() => (deletingApiKey = true)}
					>
						<Trash2 class="size-4" />
					</button>
				{/if}
			</div>

			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background rounded-lg border border-transparent p-4"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						{#if apiKey.description}
							<label for="api-key-description" class="flex-1 text-sm font-light capitalize"
								>Description</label
							>
							<input
								id="api-key-description"
								value={apiKey.description}
								class="text-input-filled mt-0.5"
								disabled
							/>
						{/if}
					</div>

					<div class="flex flex-col gap-2">
						<label for="api-key-key" class="flex-1 text-sm font-light capitalize">Key</label>
						<input
							id="api-key-key"
							value={apiKey.prefix}
							class="text-input-filled mt-0.5"
							disabled
						/>
					</div>

					<div class="flex flex-col gap-2">
						<label for="api-key-created" class="flex-1 text-sm font-light capitalize">Created</label
						>
						<input
							id="api-key-created"
							value={createdDisplay}
							class="text-input-filled mt-0.5"
							disabled
						/>
					</div>

					<div class="flex flex-col gap-2">
						<label for="api-key-last-used" class="flex-1 text-sm font-light capitalize"
							>Last Used</label
						>
						<input
							id="api-key-last-used"
							value={lastUsedDisplay}
							class="text-input-filled mt-0.5"
							disabled
						/>
					</div>

					<div class="flex flex-col gap-2">
						<label for="api-key-expires" class="flex-1 text-sm font-light capitalize">Expires</label
						>
						<input
							id="api-key-expires"
							value={expiresDisplay}
							class="text-input-filled mt-0.5"
							disabled
						/>
					</div>
				</div>
			</div>

			<div class="mt-4 flex flex-col gap-2">
				<p>
					<span class="text-lg font-semibold">Authorized Servers</span>
				</p>

				{#if resolvedServers.length > 0 || isAllServers}
					<Table
						data={isAllServers
							? [
									{
										id: 'all-mcp-servers',
										name: 'All MCP Servers',
										description: '',
										icon: '',
										exists: true
									}
								]
							: resolvedServers}
						fields={['name']}
						classes={{ row: 'px-0 py-0' }}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'name'}
								<div
									class={twMerge(
										'flex w-full items-center gap-3 px-4 py-3',
										!d.exists && 'bg-yellow-500/5'
									)}
								>
									<div class="flex-shrink-0">
										{#if d.icon}
											<img src={d.icon} alt={d.name} class="size-6" />
										{:else}
											<Server class="text-on-surface1 size-6" />
										{/if}
									</div>
									<div class="flex min-w-0 grow flex-col">
										<p
											class={twMerge(
												'truncate text-sm',
												!d.exists && 'text-on-surface1 font-light italic'
											)}
										>
											{d.name}
										</p>
										{#if d.description}
											<span class="text-on-surface1 line-clamp-1 text-xs">
												{@html stripMarkdownToText(d.description)}
											</span>
										{/if}
									</div>
								</div>
							{/if}
						{/snippet}
					</Table>
				{:else}
					<p class="text-muted">No servers authorized</p>
				{/if}
			</div>
		</div>
	</div>
{/if}

<Confirm
	msg="Are you sure you want to delete this API key?"
	show={deletingApiKey}
	onsuccess={onDelete}
	oncancel={() => (deletingApiKey = false)}
/>
