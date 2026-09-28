<script lang="ts">
	import { AdminService, type ProductTelemetryConsent } from '$lib/services';
	import { productTelemetryConsent } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import { untrack } from 'svelte';

	let {
		consent,
		saving = false,
		dirty = $bindable(false)
	}: {
		consent: ProductTelemetryConsent;
		saving?: boolean;
		dirty?: boolean;
	} = $props();

	const initialConsent = untrack(() => consent.consent);
	untrack(() => productTelemetryConsent.initialize(consent, true));
	let persistedConsent = $state<boolean | undefined>(initialConsent);
	let selectedConsent = $state<boolean | undefined>(initialConsent);

	let canSave = $derived(selectedConsent !== undefined && selectedConsent !== persistedConsent);

	$effect(() => {
		if (dirty !== canSave) dirty = canSave;
	});

	export function reset() {
		selectedConsent = persistedConsent;
	}

	export async function save() {
		if (!canSave || selectedConsent === undefined) return false;

		try {
			const response = await AdminService.updateProductTelemetryConsent(selectedConsent);
			const savedConsent = response.consent ?? selectedConsent;
			persistedConsent = savedConsent;
			selectedConsent = savedConsent;
			productTelemetryConsent.setConsent(savedConsent);
			success.add('Product analytics preference updated successfully.');
			return true;
		} catch (_err) {
			// Keep both the persisted status and unsaved selection so the administrator can retry.
			return false;
		}
	}
</script>

<div class="relative flex w-full flex-col gap-2 @container">
	<div class="paper gap-5">
		<fieldset class="flex flex-col gap-3" disabled={saving}>
			<legend class="mb-2 text-sm font-medium">Share product usage data</legend>
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
						Send product usage data to help improve Obot.
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
						Do not share product usage data.
					</span>
				</span>
			</label>
		</fieldset>
		<p class="text-xs font-light text-muted-content">
			Software update checks are separate and may send the installation ID and current version even
			when product analytics is disabled.
			<a
				class="text-link"
				href="https://docs.obot.ai/configuration/product-analytics#upgrade-checks-are-separate"
				target="_blank"
				rel="external noopener noreferrer">Learn more about update checks</a
			>.
		</p>
	</div>
</div>
