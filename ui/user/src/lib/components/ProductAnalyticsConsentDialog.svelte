<script lang="ts">
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { AdminService, Group } from '$lib/services';
	import { productTelemetryConsent, profile } from '$lib/stores';
	import {
		dismissProductAnalyticsPrompt,
		isProductAnalyticsPromptDismissed
	} from '$lib/stores/productTelemetryConsent.svelte';
	import ResponsiveDialog from './ResponsiveDialog.svelte';
	import { onMount, untrack } from 'svelte';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let dismissed = $state(true);
	let isOpen = $state(false);
	let saving = $state(false);
	let isOnProductAnalyticsSettings = $derived(
		page.url.pathname === '/admin/product-analytics' ||
			(page.url.pathname === '/admin/platform' &&
				page.url.searchParams.get('view') === 'product-analytics')
	);

	let shouldShow = $derived(
		profile.current.groups.includes(Group.ADMIN) &&
			productTelemetryConsent.available === true &&
			productTelemetryConsent.consent === undefined &&
			!isOnProductAnalyticsSettings &&
			!dismissed
	);

	onMount(() => {
		dismissed = isProductAnalyticsPromptDismissed();
	});

	$effect(() => {
		if (!browser || !dialog) return;

		if (shouldShow && !isOpen) {
			untrack(() => dialog?.open());
		} else if (!shouldShow && isOpen) {
			untrack(() => dialog?.close());
		}
	});

	function handleClose() {
		isOpen = false;
		if (shouldShow && productTelemetryConsent.consent === undefined) {
			dismissProductAnalyticsPrompt();
			dismissed = true;
		}
	}

	async function saveConsent(consent: boolean) {
		saving = true;
		try {
			const response = await AdminService.updateProductTelemetryConsent(consent);
			productTelemetryConsent.setConsent(response.consent ?? consent);
			dialog?.close();
		} catch (_err) {
			// The shared HTTP client surfaces the standard error notification.
		} finally {
			saving = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	id="product-analytics-consent-dialog"
	title="Help improve Obot"
	class="max-w-xl"
	classes={{ content: 'p-6' }}
	onOpen={() => (isOpen = true)}
	onClose={handleClose}
>
	<div class="flex flex-col gap-4 text-sm font-light">
		<p>
			Share aggregate product-usage information to help us understand how Obot is used and
			prioritize improvements.
		</p>
		<p>
			Obot does not collect prompts, messages, credentials, URLs, custom MCP server configuration
			details, authentication-provider settings beyond its type, or audit-log content as part of
			product analytics.
		</p>
		<p>
			Software update checks are separate and may send the installation ID and current version even
			when product analytics is disabled.
		</p>
		<p>You can change this choice later in Platform under Product Analytics.</p>
		<a
			class="text-link w-fit"
			href="https://docs.obot.ai/configuration/product-analytics"
			target="_blank"
			rel="external"
		>
			Learn more about Product Analytics
		</a>

		<div class="mt-2 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
			<button
				type="button"
				class="btn btn-secondary w-full sm:w-fit"
				disabled={saving}
				onclick={() => saveConsent(false)}>Don’t share</button
			>
			<button
				type="button"
				class="btn btn-primary w-full sm:w-fit"
				disabled={saving}
				onclick={() => saveConsent(true)}>Share analytics</button
			>
		</div>
	</div>
</ResponsiveDialog>
