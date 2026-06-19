<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { Group } from '$lib/services';
	import {
		AiClient,
		COMMAND_SUPPORTED_AI_CLIENTS,
		COMMON_AI_CLIENTS,
		MAGIC_LINK_SUPPORTED_AI_CLIENTS
	} from '$lib/services/user/constants';
	import { profile, userDeviceSettings } from '$lib/stores/index';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import ConnectorsView from './ConnectorsView.svelte';
	import { Server } from '@lucide/svelte';
	import { debounce } from 'es-toolkit';
	import { fade, fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();

	let setPreferredClientsDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let selectedClients = $state<AiClient[]>([]);

	let workspaceId = $derived(data.workspace?.id);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));

	let query = $derived(page.url.searchParams.get('query') || '');

	const clientsMap = $derived(new Map(COMMON_AI_CLIENTS.map((client) => [client.id, client])));
	const clients = $derived.by(() => {
		const selectedSet = new Set(selectedClients);
		return [...MAGIC_LINK_SUPPORTED_AI_CLIENTS, ...COMMAND_SUPPORTED_AI_CLIENTS]
			.sort((a, b) => a.localeCompare(b))
			.map((clientId) => ({
				...(clientsMap.get(clientId) ?? { id: clientId, icon: '', iconDark: '', alt: '' }),
				selected: selectedSet.has(clientId)
			}));
	});

	const updateSearchQuery = debounce((value: string) => {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}, 100);

	const duration = PAGE_TRANSITION_DURATION;
	let title = 'MCP Servers';
</script>

<Layout classes={{ navbar: 'bg-base-200', container: 'pt-0' }} {title}>
	{#snippet rightNavActions()}
		<button
			class="btn btn-primary"
			onclick={() => {
				selectedClients = userDeviceSettings.aiClientPreference ?? [];
				setPreferredClientsDialog?.open();
			}}
		>
			Set Preferred Client(s)
		</button>
	{/snippet}
	<div class="flex min-h-full flex-col gap-8" in:fade>
		{@render mainContent()}
	</div>
</Layout>

{#snippet mainContent()}
	<div
		class="flex flex-col"
		in:fly={{ x: 100, delay: duration, duration }}
		out:fly={{ x: -100, duration }}
	>
		<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
			<div class="mb-2">
				<Search
					class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
					value={query}
					onChange={updateSearchQuery}
					placeholder="Search servers..."
				/>
			</div>
		</div>
		<ConnectorsView id={workspaceId} entity="workspace" {query}>
			{#snippet noDataContent()}
				<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
					<Server class="text-base-content/80 size-24 opacity-25" />
					<h4 class="text-muted-content text-lg font-semibold">No created MCP servers</h4>
					<p class="text-muted-content text-sm font-light">
						{#if isAtLeastPowerUser}
							Looks like you don't have any servers created yet. <br />
							Go to
							<a
								href={resolve(`${profile.current.hasAdminAccess?.() ? '/admin' : ''}/mcp-catalog`)}
								class="text-link">MCP Management ▸ MCP Servers</a
							> to get started.
						{:else}
							There are no servers available to connect to yet. <br />
							Please check back later or contact your administrator.
						{/if}
					</p>
				</div>
			{/snippet}
		</ConnectorsView>
	</div>
{/snippet}

<ResponsiveDialog
	bind:this={setPreferredClientsDialog}
	title="Set Preferred Client(s)"
	class="md:w-sm"
>
	<fieldset class="flex flex-col gap-2">
		<label
			class={twMerge(
				'flex items-center justify-between gap-4',
				selectedClients.length === 0 ? 'btn btn-primary rounded-md!' : 'btn rounded-md! px-5!'
			)}
		>
			<div class="flex items-center gap-2">
				<span>No Preference</span>
			</div>
			<div class="flex items-center gap-2">
				<input
					id="no-preference"
					type="checkbox"
					class="checkbox text-primary-content border-0 bg-transparent disabled:opacity-100"
					checked={selectedClients.length === 0}
					disabled={selectedClients.length === 0}
					onchange={(e) => {
						e.preventDefault();
						selectedClients = [];
					}}
				/>
			</div>
		</label>
		{#each clients as client (client.id)}
			<label
				class={twMerge(
					'cursor-pointer flex items-center justify-between gap-4',
					client.selected ? 'btn btn-primary rounded-md!' : 'btn rounded-md! px-5!'
				)}
			>
				<div class="flex items-center gap-2">
					<img
						src={client?.iconDark ?? client?.icon}
						alt={client?.alt}
						class="size-4 dark:block hidden"
					/>
					<img src={client?.icon} alt={client?.alt} class="size-4 block dark:hidden" />
					<span>{client?.alt}</span>
				</div>
				<div class="flex items-center gap-2">
					<input
						id={`preferred-client-${client.id}`}
						type="checkbox"
						class="checkbox text-primary-content border-0 bg-transparent"
						checked={client.selected}
						onchange={(e) => {
							e.preventDefault();
							selectedClients = client.selected
								? selectedClients.filter((id) => id !== client.id)
								: [...selectedClients, client.id];
						}}
					/>
				</div>
			</label>
		{/each}
	</fieldset>
	<div class="flex justify-end pt-4 gap-2">
		<button class="btn btn-secondary btn-sm" onclick={() => setPreferredClientsDialog?.close()}>
			Cancel
		</button>
		<button
			class="btn btn-primary btn-sm"
			onclick={() => {
				userDeviceSettings.setAiClientPreference(selectedClients);
				setPreferredClientsDialog?.close();
				selectedClients = [];
			}}
		>
			Apply
		</button>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | MCP Servers</title>
</svelte:head>
