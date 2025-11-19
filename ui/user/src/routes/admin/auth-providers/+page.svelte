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
	import { AdminService, Role } from '$lib/services/index.js';
	import { AlertTriangle, Info } from 'lucide-svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import { twMerge } from 'tailwind-merge';
	import { darkMode, errors, profile } from '$lib/stores/index.js';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';

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

	let setupLoading = $state(false);
	let setupSignInDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let explicitOwners = $state<string[]>([]);
	let setupTempLoginUrl = $state('');

	let loading = $state(false);
	let configureError = $state<string>();

	let confirmDeconfigureAuthProvider = $state<AuthProvider>();

	const duration = PAGE_TRANSITION_DURATION;

	const prepareOwnerSetup = async () => {
		const configuredAuthProvider = authProviders.find((provider) => provider.configured);
		if (!configuredAuthProvider) return;

		const users = await AdminService.listUsers();
		const isOwnerExist = users.some((user) => user.role === Role.OWNER);

		if (isOwnerExist) return;

		// Only proceed if user is bootstrap user (not yet a real owner) and has a configured provider and owner does not exist
		if (!setupLoading && !setupTempLoginUrl) {
			configuringAuthProvider = configuredAuthProvider;
			handleOwnerSetup();
		}
	};

	$effect(() => {
		const isBootstrapUser = profile.current.isBootstrapUser?.();
		if (!isBootstrapUser) return;

		prepareOwnerSetup();
	});

	$effect(() => {
		const isBootstrapUser = profile.current.isBootstrapUser?.();
		if (!isBootstrapUser) return;

		if (!atLeastOneConfigured) return;

		const handleVisibilityChange = async () => {
			if (document.visibilityState === 'visible') {
				prepareOwnerSetup();
			}
		};

		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	async function handleOwnerSetup() {
		if (!configuringAuthProvider || setupLoading) return;
		setupLoading = true;
		try {
			await AdminService.cancelTempLogin();
		} catch (err) {
			if (err instanceof Error && err.message.includes('404')) {
				// ignore, no current temp login to cancel
			} else {
				errors.append(err);
			}
		}

		try {
			explicitOwners = (await AdminService.listExplicitRoleEmails())?.owners ?? [];
			setupTempLoginUrl = (
				await AdminService.initiateTempLogin(
					configuringAuthProvider.id,
					configuringAuthProvider.namespace
				)
			).redirectUrl;
			setupLoading = false;
			setupSignInDialog?.open();
		} catch (_) {
			// ignore
		}
	}

	async function handleAuthProviderConfigure(form: Record<string, string>) {
		if (configuringAuthProvider) {
			loading = true;
			configureError = undefined;
			try {
				await AdminService.configureAuthProvider(configuringAuthProvider.id, form);
				authProviders = await AdminService.listAuthProviders();
				adminConfigStore.updateAuthProviders(authProviders);
				providerConfigure?.close();
				if (profile.current.isBootstrapUser?.()) {
					await handleOwnerSetup();
				}
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

<Layout title="Auth Providers">
	<div class="my-4" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-8">
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
					disableConfigure={atLeastOneConfigured && !authProvider.configured}
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
							} else {
								// no credentials set, set initial default value for allowed domains
								configuringAuthProviderValues = {
									OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*'
								};
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

<ResponsiveDialog bind:this={setupSignInDialog} class="w-md">
	{#snippet titleContent()}
		<h3 class="text-lg font-semibold">Next Step: Owner Login Setup</h3>
	{/snippet}

	<div class="flex flex-col gap-4">
		{#if explicitOwners.length > 0}
			<p>You'll need to continue setup with an owner account.</p>
			<p>The following user(s) have been explicitly assigned the Owner role:</p>
			<ul class="list-disc px-8">
				{#each explicitOwners as owner (owner)}
					<li>{owner}</li>
				{/each}
			</ul>
			<p>
				Log in into the system as one of the explicit owners -- you'll be redirected back to the
				admin panel after authenticating.
			</p>
			<p>
				Or log into a different account with your configured auth provider. After authentication,
				you'll be asked to confirm the owner addition before proceeding.
			</p>
		{:else}
			<p>
				You'll need to set up an initial owner for the system. Login with your configured auth
				provider to continue.
			</p>
		{/if}

		<div class="my-4 flex flex-col gap-2">
			<a class="button-auth group" href={setupTempLoginUrl}>
				{#if configuringAuthProvider?.icon}
					<img
						class="h-6 w-6 rounded-full bg-transparent p-1 dark:bg-gray-600"
						src={configuringAuthProvider.icon}
						alt={configuringAuthProvider.name}
					/>
					<span class="text-center text-sm font-light">
						Continue with {configuringAuthProvider.name}
					</span>
				{/if}
			</a>
		</div>
	</div>
</ResponsiveDialog>

<svelte:head>
	<title>Obot | Auth Providers</title>
</svelte:head>
