<script lang="ts">
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import {
		CommonAuthProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders
	} from '$lib/constants';
	import { fade } from 'svelte/transition';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';
	import type { AuthProvider } from '$lib/services/admin/types.js';
	import { AdminService } from '$lib/services/index.js';
	import { AlertTriangle, Info } from 'lucide-svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { twMerge } from 'tailwind-merge';
	import { darkMode, profile } from '$lib/stores/index.js';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';

	let { data } = $props();
	let { authProviders: initialAuthProviders } = data;
	let authProviders = $state(initialAuthProviders);

	function sortAuthProviders(authProviders: AuthProvider[]) {
		return [...authProviders].sort((a, b) => {
			const preferredOrder: string[] = [
				CommonAuthProviderIds.GOOGLE,
				CommonAuthProviderIds.GITHUB,
				CommonAuthProviderIds.OKTA
			];
			const aIndex = preferredOrder.indexOf(a.id);
			const bIndex = preferredOrder.indexOf(b.id);

			// If both providers are in preferredOrder, sort by their order
			if (aIndex !== -1 && bIndex !== -1) {
				return aIndex - bIndex;
			}

			// If only a is in preferredOrder, it comes first
			if (aIndex !== -1) return -1;
			// If only b is in preferredOrder, it comes first
			if (bIndex !== -1) return 1;

			// For all other providers, sort alphabetically by name
			return a.name.localeCompare(b.name);
		});
	}
	let sortedAuthProviders = $derived(sortAuthProviders(authProviders));
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let configuringAuthProvider = $state<AuthProvider>();
	let configuringAuthProviderValues = $state<Record<string, string>>();
	let atLeastOneConfigured = $derived(authProviders.some((provider) => provider.configured));

	let loading = $state(false);
	let configureError = $state<string>();

	let confirmDeconfigureAuthProvider = $state<AuthProvider>();

	const duration = PAGE_TRANSITION_DURATION;

	async function handleAuthProviderConfigure(form: Record<string, string>) {
		if (configuringAuthProvider) {
			loading = true;
			configureError = undefined;
			try {
				await AdminService.configureAuthProvider(configuringAuthProvider.id, form);
				authProviders = await AdminService.listAuthProviders();
				adminConfigStore.updateAuthProviders(authProviders);
				providerConfigure?.close();
			} catch (err: unknown) {
				if (err instanceof Error) {
					const errorMessageMatch = err.message.match(/{"error":\s*"(.*?)"}/);
					if (errorMessageMatch) {
						const errorMessage = JSON.parse(errorMessageMatch[0]).error;
						configureError = errorMessage;
					}
				} else {
					configureError = 'Failed to configure auth provider';
				}
			} finally {
				loading = false;
			}
		}
	}
</script>

<Layout>
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
			<h1 class="text-2xl font-semibold">Auth Providers</h1>
			{#if !atLeastOneConfigured}
				<div class="notification-alert flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">No Auth Providers Configured!</p>
					</div>
					<span class="text-sm font-light break-all">
						To finish setting up Obot, you'll need to configure an Auth Provider. Select one below
						to get started!
					</span>
				</div>
			{/if}
		</div>
		<div class="grid grid-cols-1 gap-4 py-8 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each sortedAuthProviders as authProvider (authProvider.id)}
				<ProviderCard
					provider={authProvider}
					recommended={RecommendedModelProviders.includes(authProvider.id)}
					onConfigure={async () => {
						configuringAuthProvider = authProvider;
						try {
							configuringAuthProviderValues = await AdminService.revealAuthProvider(
								authProvider.id
							);
						} catch (err) {
							// if 404, ignore, it means no credentials are set
							if (err instanceof Error && !err.message.includes('404')) {
								console.error('An error occurred while revealing auth provider credentials', err);
							}
						}
						providerConfigure?.open();
					}}
					onDeconfigure={async () => {
						confirmDeconfigureAuthProvider = authProvider;
					}}
					readonly={profile.current.isAdminReadonly?.()}
				/>
			{/each}
		</div>
	</div>
</Layout>

<ProviderConfigure
	bind:this={providerConfigure}
	provider={configuringAuthProvider}
	values={configuringAuthProviderValues}
	onConfigure={handleAuthProviderConfigure}
	{loading}
	error={configureError}
	readonly={profile.current.isAdminReadonly?.()}
>
	{#snippet note()}
		{@const callbackUrl = window.location.protocol + '//' + window.location.host + '/'}
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<p class="flex flex-wrap items-center gap-2">
					Note: the callback URL for this auth provider is
					<CopyButton
						showTextLeft
						buttonText={callbackUrl}
						text={callbackUrl}
						classes={{
							button: 'group'
						}}
						class="group-hover:text-white"
					/>
				</p>
			</div>
		</div>
	{/snippet}
</ProviderConfigure>

<Confirm
	show={!!confirmDeconfigureAuthProvider}
	{loading}
	onsuccess={async () => {
		if (confirmDeconfigureAuthProvider) {
			loading = true;
			await AdminService.deconfigureAuthProvider(confirmDeconfigureAuthProvider.id);
			authProviders = await AdminService.listAuthProviders();
			adminConfigStore.updateAuthProviders(authProviders);
			confirmDeconfigureAuthProvider = undefined;
			loading = false;
		}
	}}
	oncancel={() => (confirmDeconfigureAuthProvider = undefined)}
>
	{#snippet title()}
		<div class="mb-5 flex items-center gap-2">
			<img
				src={darkMode.isDark && confirmDeconfigureAuthProvider?.iconDark
					? confirmDeconfigureAuthProvider.iconDark
					: confirmDeconfigureAuthProvider?.icon}
				alt={confirmDeconfigureAuthProvider?.name}
				class={twMerge(
					'size-6 rounded-sm p-0.5',
					!confirmDeconfigureAuthProvider?.iconDark && 'bg-surface1 dark:bg-gray-600'
				)}
			/>
			<h3 class="text-lg font-semibold">Deconfigure {confirmDeconfigureAuthProvider?.name}</h3>
		</div>
	{/snippet}
	{#snippet note()}
		<div class="mb-5 flex flex-col gap-2 text-left">
			<p class="text-sm font-light">
				Deconfiguring this auth provider will sign out all users who are using it and reset it to
				its unconfigured state. You will need to set up the auth provider once again to use it.
			</p>
			<p class="text-sm font-light">
				Are you sure you want to deconfigure <b>Google</b>?
			</p>
		</div>
	{/snippet}
</Confirm>

<svelte:head>
	<title>Obot | Auth Providers</title>
</svelte:head>
