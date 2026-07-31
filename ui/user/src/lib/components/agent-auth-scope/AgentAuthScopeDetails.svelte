<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { stripMarkdownToText } from '$lib/markdown';
	import { API_KEY_CREATABLE_CAPABILITIES, type APIKey } from '$lib/services/api-keys/types';
	import {
		compileAvailableMcpServers,
		getMCPDisplayName,
		isDeprecatedMCPServer
	} from '$lib/services/user/mcp';
	import { mcpServersAndEntries, profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import Confirm from '../Confirm.svelte';
	import McpDeprecatedNotice from '../mcp/McpDeprecatedNotice.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import { KeyRound, Server, Trash2 } from '@lucide/svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		agentAuthScope?: APIKey & { prefix: string };
		onDelete: () => void;
	}

	let { agentAuthScope, onDelete }: Props = $props();
	let deletingAgentAuthScope = $state(false);
	let saving = $state(false);

	let mcpServers = $derived(
		compileAvailableMcpServers(
			mcpServersAndEntries.current.servers,
			mcpServersAndEntries.current.userConfiguredServers
		)
	);

	let serverMap = $derived(new Map(mcpServers.map((s) => [s.id, s])));

	let isAllServers = $derived(agentAuthScope?.mcpServerIds?.includes('*') ?? false);

	let resolvedServers = $derived.by(() => {
		if (!agentAuthScope?.mcpServerIds || isAllServers) return [];
		return agentAuthScope.mcpServerIds.map((id) => {
			const server = serverMap.get(id);
			return {
				id,
				name: getMCPDisplayName(server, '(Deleted)'),
				description: server?.manifest.description,
				icon: server?.manifest.icon,
				exists: !!server,
				deprecated: isDeprecatedMCPServer(server)
			};
		});
	});

	let createdDisplay = $derived(
		agentAuthScope ? formatTimeAgo(agentAuthScope.createdAt).relativeTime : ''
	);
	let lastUsedDisplay = $derived(
		agentAuthScope?.lastUsedAt ? formatTimeAgo(agentAuthScope.lastUsedAt).relativeTime : 'Never'
	);
	let expiresDisplay = $derived(
		agentAuthScope?.expiresAt ? formatTimeUntil(agentAuthScope.expiresAt).relativeTime : 'Never'
	);
	let mcpServerData = $derived(
		isAllServers
			? [
					{
						id: 'all-mcp-servers',
						name: 'All MCP Servers',
						description: '',
						icon: '',
						exists: true,
						deprecated: false
					}
				]
			: resolvedServers
	);

	const duration = PAGE_TRANSITION_DURATION;
	const title = $derived(agentAuthScope?.name || 'Agent Auth Scope');
</script>

{#if agentAuthScope}
	<div
		class="flex h-full w-full flex-col gap-4"
		out:fly={{ x: 100, duration }}
		in:fly={{ x: 100, delay: duration }}
	>
		<div
			class="flex grow flex-col gap-4 @container"
			out:fly={{ x: -100, duration }}
			in:fly={{ x: -100 }}
		>
			<section
				class="paper p-4 flex-row flex-wrap @md:flex-nowrap items-center gap-4 justify-between"
			>
				<div class="flex items-center gap-4">
					<div class="p-2 @md:p-4">
						<KeyRound class="size-8" />
					</div>
					<div class="text-sm flex flex-col gap-0.5">
						<h1 class="text-xl font-semibold">{title}</h1>
						<p>{agentAuthScope.description}</p>
						<p><b>Last Used:</b> {lastUsedDisplay}</p>
						<p><b>Expires:</b> {expiresDisplay}</p>
						<p class="text-muted-content font-light">
							Created {createdDisplay}
						</p>
					</div>
				</div>
				{#if agentAuthScope.userId.toString() === profile.current.id}
					<div class="flex w-full @md:w-auto justify-end">
						<IconButton
							class=""
							variant="danger2"
							tooltip={{ text: `Delete ${title}` }}
							disabled={saving}
							onclick={() => (deletingAgentAuthScope = true)}
						>
							<Trash2 class="size-4" />
						</IconButton>
					</div>
				{/if}
			</section>

			<section class="paper flex flex-col gap-2 p-4">
				<p>
					<span class="text-lg font-semibold">MCP Servers</span>
				</p>

				<ul
					class="list-none bg-base-200 dark:bg-base-300 default-scrollbar-thin flex max-h-64 flex-col overflow-y-auto rounded-lg"
				>
					{#if mcpServerData.length === 0}
						<li class="text-muted-content flex items-center justify-center py-8 text-sm">
							No MCP servers
						</li>
					{:else}
						{#each mcpServerData as server (server.id)}
							<li class="flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors">
								<div class="flex w-full items-center gap-3 overflow-hidden">
									<div class="shrink-0">
										{#if server.icon}
											<img src={server.icon} alt={server.name} class="size-6" />
										{:else}
											<Server class="text-muted-content size-6" />
										{/if}
									</div>
									<div class="flex min-w-0 grow flex-col">
										<div class="flex items-center gap-2">
											<p class="min-w-0 truncate text-sm">{server.name}</p>
											<McpDeprecatedNotice deprecated={server.deprecated} />
										</div>
										{#if server.description}
											<span class="text-muted-content line-clamp-1 text-xs">
												{stripMarkdownToText(server.description)}
											</span>
										{/if}
									</div>
								</div>
							</li>
						{/each}
					{/if}
				</ul>
			</section>

			<section class="paper gap-2 p-4">
				<p class="text-lg font-semibold" id="agent-auth-scope-scopes">API Scopes</p>
				<div class="flex flex-col gap-2" role="group" aria-labelledby="agent-auth-scope-scopes">
					{#each API_KEY_CREATABLE_CAPABILITIES as capability (capability.key)}
						<label
							class={twMerge(
								'bg-base-200 flex items-center gap-3 rounded-lg border border-transparent p-3',
								agentAuthScope[capability.key] && 'bg-primary/10 border-primary'
							)}
						>
							<input
								type="checkbox"
								bind:checked={agentAuthScope[capability.key]}
								class={twMerge(
									'checkbox checkbox-xs rounded-sm',
									agentAuthScope[capability.key] && 'checkbox-primary'
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

			<section class="paper gap-2 p-4">
				<p class="text-lg font-semibold" id="agent-auth-scope-keys">API Keys</p>
				<div class="flex flex-col gap-2" role="group" aria-labelledby="agent-auth-scope-keys">
					<div class="bg-base-200 flex items-center gap-3 rounded-lg p-3 text-sm">
						{agentAuthScope.prefix}
					</div>
				</div>
			</section>
		</div>
	</div>
{/if}

<Confirm
	msg={`Are you sure you want to delete "${title}"?`}
	show={deletingAgentAuthScope}
	onsuccess={onDelete}
	oncancel={() => (deletingAgentAuthScope = false)}
/>
