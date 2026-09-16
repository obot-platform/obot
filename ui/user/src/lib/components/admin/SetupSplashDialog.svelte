<script lang="ts">
	import { page } from '$app/state';
	import Loading from '$lib/icons/Loading.svelte';
	import { getSeenTimestamp, markSeenTimestamp } from '$lib/localstate';
	import { AdminService, Group } from '$lib/services';
	import { productTelemetryConsent, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import {
		deferProductAnalyticsConsent,
		isProductAnalyticsConsentDeferred
	} from '$lib/stores/productTelemetryConsent.svelte';
	import setupSplash from '$lib/stores/setupSplash.svelte';
	import { goto, setUrlParamAndUpdateUrl } from '$lib/url';
	import Logo from '../Logo.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { onDestroy, onMount } from 'svelte';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let loading = $state(false);
	let shareProductUsage = $state(true);
	let productAnalyticsDeferred = $state(true);
	let splashOpened = $state(false);

	// Block other onboarding dialogs until this splash has either opened or decided not to.
	setupSplash.blocking = true;
	onDestroy(() => {
		setupSplash.blocking = false;
	});

	const authProviderPath = '/identity-access';
	const modelProviderPath = '/models?view=model-providers';
	const seenSplashDialogKey = 'seenSplashDialog';

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

	function releaseSplashBlock() {
		splashOpened = false;
		setupSplash.blocking = false;
	}

	$effect(() => {
		if (!profile.current.loaded || profile.current.unauthorized) {
			return;
		}

		if (!storeData.lastFetched) {
			const mightSeeSplash =
				profile.current.hasAdminAccess?.() ||
				profile.current.isBootstrapUser?.() ||
				profile.current.groups.includes(Group.OWNER);
			if (!mightSeeSplash) {
				setupSplash.blocking = false;
			}
			return;
		}

		const { seenAt: firstTimeViewed } = getSeenTimestamp(
			seenSplashDialogKey,
			profile.current.created
		);

		const isOwner = profile.current.groups.includes(Group.OWNER);
		const needsSetup =
			!firstTimeViewed &&
			(isBootstrapUser || isOwner) &&
			(!isAuthProviderConfigured || requiresModelProviderConfiguration || !storeData.eulaAccepted);
		if (needsSetup || needsProductAnalyticsConsent) {
			splashOpened = true;
			setupSplash.blocking = true;
			dialog?.open();
			return;
		}

		if (!splashOpened) {
			setupSplash.blocking = false;
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

	async function finishOnboarding() {
		if (isBootstrapUser) {
			if (isOnAuthProvidersPage) {
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
	}

	async function handleContinue() {
		loading = true;
		try {
			await handleProductAnalyticsConsent();
			await handleAcceptEula();
			markSeenTimestamp(seenSplashDialogKey);
			dialog?.close();
			releaseSplashBlock();
			await finishOnboarding();
		} finally {
			loading = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	hideClose
	disableClickOutside
	class="text-md md:w-sm max-w-full rounded-lg"
	classes={{
		content: 'p-4 justify-center'
	}}
	onClose={releaseSplashBlock}
>
	<div class="flex w-full items-center justify-center">
		<Logo class="size-18" />
	</div>
	<h2 class="mb-8 text-center text-2xl font-semibold">Welcome to Obot!</h2>

	<div class="w-fit self-center px-4">
		{#if !version.current.authEnabled}
			<p class="mb-4">
				<span class="text-muted-content">Auth is disabled.</span>
				<a
					href="https://docs.obot.ai/installation/enabling-authentication"
					rel="external noopener noreferrer"
					target="_blank"
					class="text-link">Learn more</a
				>
			</p>
		{/if}
		<p>By continuing, you agree to the following:</p>

		<div class="flex items-center gap-2 text-sm pt-4">
			<div class="mx-2">&#8226;</div>
			<span>
				I agree to Obot's
				<a
					href="https://obot.ai/eul"
					rel="external noopener noreferrer"
					target="_blank"
					class="text-link">EULA</a
				>
			</span>
		</div>
		{#if needsProductAnalyticsConsent}
			<div class="flex items-start gap-2 pt-4 text-sm">
				<input
					id="share-product-usage"
					type="checkbox"
					class="checkbox checkbox-sm shrink-0 checked:checkbox-primary"
					bind:checked={shareProductUsage}
					disabled={loading}
				/>
				<span class="italic">
					<label for="share-product-usage">
						I agree to share my product usage data to help improve Obot (optional)
					</label>
					<br />
					<a
						href="https://docs.obot.ai/configuration/product-analytics"
						rel="external noopener noreferrer"
						target="_blank"
						class="text-link">Learn more</a
					>
				</span>
			</div>
		{/if}
	</div>

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
</ResponsiveDialog>
