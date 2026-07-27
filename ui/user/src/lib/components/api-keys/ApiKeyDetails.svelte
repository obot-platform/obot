<script lang="ts">
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { stripMarkdownToText } from '$lib/markdown';
	import { API_KEY_CAPABILITIES, type APIKey } from '$lib/services/api-keys/types';
	import { compileAvailableMcpServers, getMCPDisplayName } from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import Confirm from '../Confirm.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import { Server, Trash2 } from '@lucide/svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

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
				name: getMCPDisplayName(server, '(Deleted)'),
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
	const title = $derived(apiKey?.name || 'Agent Auth Scope');
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
					{title}
				</h1>
				{#if apiKey.userId.toString() === profile.current.id}
					<IconButton
						variant="danger2"
						tooltip={{ text: `Delete ${title}` }}
						disabled={saving}
						onclick={() => (deletingApiKey = true)}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			</div>

			<section class="paper">
				{#if apiKey.description}
					<div class="flex flex-col gap-2">
						<label for="api-key-description" class="flex-1 text-sm font-light capitalize"
							>Description</label
						>
						<input
							id="api-key-description"
							value={apiKey.description}
							class="text-input-filled mt-0.5"
							disabled
						/>
					</div>
				{/if}

				<div class="flex flex-col gap-2">
					<label for="api-key-key" class="flex-1 text-sm font-light capitalize">Key</label>
					<input id="api-key-key" value={apiKey.prefix} class="text-input-filled mt-0.5" disabled />
				</div>

				<div class="flex flex-col gap-2">
					<label for="api-key-created" class="flex-1 text-sm font-light capitalize">Created</label>
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
					<label for="api-key-expires" class="flex-1 text-sm font-light capitalize">Expires</label>
					<input
						id="api-key-expires"
						value={expiresDisplay}
						class="text-input-filled mt-0.5"
						disabled
					/>
				</div>
			</section>

			<section class="flex flex-col gap-2">
				<p class="text-lg font-semibold">MCP Servers</p>

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
										!d.exists && 'bg-warning/5'
									)}
								>
									<div class="shrink-0">
										{#if d.icon}
											<img src={d.icon} alt={d.name} class="size-6" />
										{:else}
											<Server class="text-muted-content size-6" />
										{/if}
									</div>
									<div class="flex min-w-0 grow flex-col">
										<p
											class={twMerge(
												'truncate text-sm',
												!d.exists && 'text-muted-content font-light italic'
											)}
										>
											{d.name}
										</p>
										{#if d.description}
											<span class="text-muted-content line-clamp-1 text-xs">
												{stripMarkdownToText(d.description)}
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
			</section>

			<section class="paper gap-2 p-4">
				<p class="text-lg font-semibold" id="api-key-scopes">API Scopes</p>
				<div class="flex flex-col gap-2" role="group" aria-labelledby="api-key-scopes">
					{#each API_KEY_CAPABILITIES as capability (capability.key)}
						<label
							class={twMerge(
								'bg-base-200 flex items-center gap-3 rounded-lg border border-base-400 p-3',
								apiKey[capability.key] && 'bg-primary/10 border-primary'
							)}
						>
							<input
								type="checkbox"
								bind:checked={apiKey[capability.key]}
								class={twMerge(
									'checkbox checkbox-xs rounded-sm',
									apiKey[capability.key] && 'checkbox-primary'
								)}
								disabled
							/>
							<div class="flex flex-col gap-0.5">
								<span class="text-sm font-medium">{capability.label}</span>
								<span class="input-description">{capability.description}</span>
							</div>
						</label>
					{/each}
				</div>
			</section>

			<section class="flex flex-col gap-2">
				<p class="text-lg font-semibold">API Keys</p>
				<Table
					data={[{ id: apiKey.id, prefix: apiKey.prefix }]}
					fields={['prefix']}
					headers={[{ title: 'Key', property: 'prefix' }]}
				/>
			</section>
		</div>
	</div>
{/if}

<Confirm
	msg={`Are you sure you want to delete "${title}"?`}
	show={deletingApiKey}
	onsuccess={onDelete}
	oncancel={() => (deletingApiKey = false)}
/>
