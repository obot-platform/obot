<script lang="ts">
	import { page } from '$app/state';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, Group } from '$lib/services';
	import { productTelemetryConsent, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import {
		deferProductAnalyticsConsent,
		isProductAnalyticsConsentDeferred
	} from '$lib/stores/productTelemetryConsent.svelte';
	import { goto, setUrlParamAndUpdateUrl } from '$lib/url';
	import Logo from '../Logo.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { CircleCheckBig } from '@lucide/svelte';
	import { onMount, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let loading = $state(false);
	let shareProductUsage = $state(true);
	let productAnalyticsDeferred = $state(true);

	const authProviderPath = '/identity-access';
	const modelProviderPath = '/models?view=model-providers';

	const storeData = $derived($adminConfigStore);
	const isAuthProviderConfigured = $derived(
		version.current.authEnabled ? storeData.authProviderConfigured : true
	);
	const view = $derived(page.url.searchParams.get('view'));
	const requiresModelProviderConfiguration = $derived(
		version.current.agentsEnabled !== false && !storeData.modelProviderConfigured
	);
	const isOnAuthProvidersPage = $derived(
		page.url.pathname === authProviderPath && view === 'auth-providers'
	);
	const isOnProductAnalyticsSettings = $derived(
		page.url.pathname === '/admin/product-analytics' ||
			(page.url.pathname === '/admin/platform' && view === 'product-analytics')
	);
	const isBootstrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);
	const needsProductAnalyticsConsent = $derived(
		profile.current.groups.includes(Group.ADMIN) &&
			productTelemetryConsent.available === true &&
			productTelemetryConsent.consent === undefined &&
			!isOnProductAnalyticsSettings &&
			!productAnalyticsDeferred
	);

	onMount(() => {
		productAnalyticsDeferred = isProductAnalyticsConsentDeferred();
	});

	$effect(() => {
		if (profile.current.loaded && !profile.current.unauthorized && storeData.lastFetched) {
			const created = profile.current.created ? new Date(profile.current.created) : null;
			let firstTimeViewed = localStorage.getItem('seenSplashDialog')
				? new Date(localStorage.getItem('seenSplashDialog')!)
				: null;

			// the user is newer than the seenSplashDialog set, likely case of fresh install & revisiting with browser
			if (created && firstTimeViewed && created > firstTimeViewed) {
				localStorage.removeItem('seenSplashDialog');
				firstTimeViewed = null;
			}

			const isOwner = profile.current.groups.includes(Group.OWNER);
			const needsSetup =
				!firstTimeViewed &&
				(isBootstrapUser || isOwner) &&
				(!isAuthProviderConfigured ||
					requiresModelProviderConfiguration ||
					!storeData.eulaAccepted);
			if (needsSetup || needsProductAnalyticsConsent) {
				dialog?.open();
			}
		}
	});

	async function handleAcceptEula() {
		if (storeData.eulaAccepted) return;
		const response = await AdminService.acceptEula();
		adminConfigStore.updateEula(response.accepted);
	}

	async function handleProductAnalyticsConsent() {
		if (!needsProductAnalyticsConsent) return;

		try {
			const response = await AdminService.updateProductTelemetryConsent(shareProductUsage);
			productTelemetryConsent.setConsent(response.consent ?? shareProductUsage);
		} catch (_err) {
			// The shared HTTP client surfaces the standard error notification. Do not block onboarding
			// for an optional analytics preference; ask again after the next session begins.
			deferProductAnalyticsConsent();
			productAnalyticsDeferred = true;
		}
	}

	async function handleContinue() {
		loading = true;
		try {
			await handleProductAnalyticsConsent();
			await handleAcceptEula();
			localStorage.setItem('seenSplashDialog', new Date().toISOString());

			if (isBootstrapUser) {
				if (isOnAuthProvidersPage) {
					dialog?.close();
					setUrlParamAndUpdateUrl(page.url, 'provider', 'local-auth-provider');
					return;
				}

				if (!isAuthProviderConfigured) {
					goto(`${authProviderPath}?view=auth-providers&provider=local-auth-provider`);
				} else if (requiresModelProviderConfiguration) {
					goto(modelProviderPath);
				}
			} else if (requiresModelProviderConfiguration && page.url.pathname !== modelProviderPath) {
				goto(modelProviderPath);
			}

			dialog?.close();
		} finally {
			loading = false;
		}
	}
</script>

<ResponsiveDialog bind:this={dialog} hideClose disableClickOutside class="text-md w-sm">
	<div class="flex w-full items-center justify-center">
		<Logo class="size-18" />
	</div>
	<h2 class="mb-8 text-center text-2xl font-semibold">Welcome to Obot!</h2>

	<div class="w-fit self-center">
		{#if !isAuthProviderConfigured || requiresModelProviderConfiguration}
			{#if isBootstrapUser}
				<p>Before using Obot, you'll need to:</p>
			{:else}
				<p class="text-center">
					You're almost there! You just need to configure your model provider.
				</p>
			{/if}

			<ul class="checklist">
				{@render renderChecklistItem(
					'Setup an Authentication Provider',
					isAuthProviderConfigured,
					authDisabledNote
				)}
				{#if version.current.agentsEnabled !== false}
					{@render renderChecklistItem('Setup a Model Provider', storeData.modelProviderConfigured)}
				{/if}
			</ul>
		{/if}

		{#if needsProductAnalyticsConsent}
			<div class="flex items-start gap-2 pt-4 text-sm">
				<input
					id="share-product-usage"
					type="checkbox"
					class="checkbox checkbox-sm mt-0.5 shrink-0"
					bind:checked={shareProductUsage}
					disabled={loading}
				/>
				<span>
					<label for="share-product-usage">Share product usage data to help improve Obot.</label>
					<a
						href="https://docs.obot.ai/configuration/product-analytics"
						rel="external noopener noreferrer"
						target="_blank"
						class="text-link">Learn more</a
					>
				</span>
			</div>
		{/if}

		<p class="pt-4">
			By continuing, you agree to Obot's <a
				href="https://obot.ai/eul"
				rel="external noopener noreferrer"
				target="_blank"
				class="text-link">EULA</a
			>
		</p>
	</div>

	{#if isBootstrapUser}
		<button
			class="btn btn-primary mt-8 flex justify-center text-center"
			disabled={loading}
			onclick={handleContinue}
		>
			{#if loading}
				<Loading class="size-4" />
			{:else}
				Get Started
			{/if}
		</button>
	{:else}
		<button
			class="btn btn-primary mt-8 flex justify-center text-center"
			disabled={loading}
			onclick={handleContinue}
		>
			{#if loading}
				<Loading class="size-4" />
			{:else}
				Continue
			{/if}
		</button>
	{/if}
</ResponsiveDialog>

{#snippet authDisabledNote()}
	{#if !version.current.authEnabled}
		<p class="mt-1 text-sm">
			<span class="text-muted-content">Auth is disabled.</span>
			<a
				href="https://docs.obot.ai/installation/enabling-authentication"
				rel="external noopener noreferrer"
				target="_blank"
				class="text-link">Learn more</a
			>
		</p>
	{/if}
{/snippet}

{#snippet renderChecklistItem(label: string, isChecked: boolean, note?: Snippet)}
	<li>
		<span
			class={twMerge('flex items-center gap-1', isChecked ? 'text-muted-content line-through' : '')}
		>
			{label}
			{#if isChecked}
				<CircleCheckBig class="size-5 text-success" />
			{/if}
		</span>
		{#if note}
			{@render note()}
		{/if}
	</li>
{/snippet}

<style lang="postcss">
	.checklist {
		padding-left: 1rem;
		margin-top: 0.5rem;
		list-style-type: disc;
		li {
			margin-bottom: 0.5rem;
			gap: 0.5rem;
		}
	}
</style>
