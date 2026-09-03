<script lang="ts">
	import { AdminService, type ProductTelemetryConsent } from '$lib/services';
	import { productTelemetryConsent } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { untrack } from 'svelte';

	let { consent }: { consent: ProductTelemetryConsent } = $props();
	const initialConsent = untrack(() => consent.consent);
	untrack(() => productTelemetryConsent.initialize(consent, true));
	let persistedConsent = $state<boolean | undefined>(initialConsent);
	let selectedConsent = $state<boolean | undefined>(initialConsent);
	let saving = $state(false);

	let status = $derived(
		persistedConsent === true
			? 'Enabled'
			: persistedConsent === false
				? 'Disabled'
				: 'No decision recorded'
	);
	let canSave = $derived(selectedConsent !== undefined && selectedConsent !== persistedConsent);

	async function handleSave() {
		if (!canSave || selectedConsent === undefined) return;

		saving = true;
		try {
			const response = await AdminService.updateProductTelemetryConsent(selectedConsent);
			const savedConsent = response.consent ?? selectedConsent;
			persistedConsent = savedConsent;
			selectedConsent = savedConsent;
			productTelemetryConsent.setConsent(savedConsent);
			success.add('Product analytics preference updated successfully.');
		} catch (_err) {
			// Keep both the persisted status and unsaved selection so the administrator can retry.
		} finally {
			saving = false;
		}
	}
</script>

<div class="relative flex h-full w-full flex-col gap-4 @container pt-4">
	<div class="paper gap-5">
		<div class="flex flex-col gap-2 text-sm font-light">
			<p>
				Share aggregate product-usage information to help us understand how Obot is used and
				prioritize improvements.
			</p>
			<p>
				Obot does not collect prompts, messages, credentials, URLs, custom MCP server configuration
				details, authentication-provider settings beyond its type, or audit-log content as part of
				product analytics.
			</p>
		</div>

		<div class="divider my-0"></div>

		<div>
			<p class="text-sm font-medium">Current status</p>
			<p
				class="mt-1 text-sm font-light text-muted-content"
				aria-label="Current product analytics status"
			>
				{status}
			</p>
		</div>

		<div class="divider my-0"></div>

		<fieldset class="flex flex-col gap-3">
			<legend class="mb-2 text-sm font-medium">Share product analytics</legend>
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="product-analytics-consent"
					aria-label="Enable product analytics"
					checked={selectedConsent === true}
					onchange={() => (selectedConsent = true)}
				/>
				<span>
					<span class="block text-sm font-medium">Enabled</span>
					<span class="block text-xs font-light text-muted-content">
						Share aggregate product-usage metrics to help improve Obot.
					</span>
				</span>
			</label>
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="product-analytics-consent"
					aria-label="Disable product analytics"
					checked={selectedConsent === false}
					onchange={() => (selectedConsent = false)}
				/>
				<span>
					<span class="block text-sm font-medium">Disabled</span>
					<span class="block text-xs font-light text-muted-content">
						Do not send product-usage analytics reports.
					</span>
				</span>
			</label>
		</fieldset>

		<p class="text-xs font-light text-muted-content">
			This setting does not affect the independent software update check. See the
			<a
				class="text-link"
				href="https://docs.obot.ai/configuration/product-analytics"
				target="_blank"
				rel="external">Product Analytics documentation</a
			>
			for details.
		</p>
	</div>

	<div class="flex grow"></div>
	<div
		class="bg-base-200 text-muted-content dark:bg-base-100 sticky bottom-0 left-0 z-50 flex w-full justify-end py-4"
	>
		<button
			type="button"
			class="btn btn-primary text-sm"
			disabled={!canSave || saving}
			onclick={handleSave}>Save</button
		>
	</div>
</div>
