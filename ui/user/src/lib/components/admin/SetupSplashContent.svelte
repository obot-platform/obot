<script lang="ts">
	import { page } from '$app/state';
	import { SEEN_SPLASH_DIALOG_KEY } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { markSeenTimestamp } from '$lib/localstate';
	import { AdminService, Group } from '$lib/services';
	import { productTelemetryConsent, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import {
		deferProductAnalyticsConsent,
		isProductAnalyticsConsentDeferred
	} from '$lib/stores/productTelemetryConsent.svelte';
	import Logo from '../Logo.svelte';
	import { onMount } from 'svelte';

	let { onContinue }: { onContinue?: () => void | Promise<void> } = $props();

	let loading = $state(false);
	let shareProductUsage = $state(true);
	let productAnalyticsDeferred = $state(true);

	const storeData = $derived($adminConfigStore);
	const isOnProductAnalyticsSettings = $derived(
		page.url.pathname === '/admin/product-analytics' ||
			(page.url.pathname === '/admin/platform' &&
				page.url.searchParams.get('view') === 'product-analytics')
	);
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
			markSeenTimestamp(SEEN_SPLASH_DIALOG_KEY);
			await onContinue?.();
		} finally {
			loading = false;
		}
	}
</script>

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

	<div class="flex items-center gap-2 pt-4 text-sm">
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
