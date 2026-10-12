<script lang="ts">
	import { m } from '$lib/i18n';
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
			success.add(m.platform_settings_product_analytics_updated());
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
			<legend class="mb-2 text-sm font-medium"
				>{m.platform_settings_product_analytics_share_legend()}</legend
			>
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="product-analytics-consent"
					aria-label={m.platform_settings_product_analytics_enable_aria()}
					checked={selectedConsent === true}
					onchange={() => (selectedConsent = true)}
				/>
				<span>
					<span class="block text-sm font-medium">{m.core_status_enabled()}</span>
					<span class="block text-xs font-light text-muted-content">
						{m.platform_settings_product_analytics_enabled_description()}
					</span>
				</span>
			</label>
			<label class="flex cursor-pointer items-start gap-3 rounded-lg border border-base-300 p-3">
				<input
					type="radio"
					class="radio radio-primary radio-sm mt-0.5"
					name="product-analytics-consent"
					aria-label={m.platform_settings_product_analytics_disable_aria()}
					checked={selectedConsent === false}
					onchange={() => (selectedConsent = false)}
				/>
				<span>
					<span class="block text-sm font-medium">{m.core_status_disabled()}</span>
					<span class="block text-xs font-light text-muted-content">
						{m.platform_settings_product_analytics_disabled_description()}
					</span>
				</span>
			</label>
		</fieldset>
		<p class="text-xs font-light text-muted-content">
			{m.platform_settings_product_analytics_update_checks()}
			<a
				class="text-link"
				href="https://docs.obot.ai/configuration/product-analytics#upgrade-checks-are-separate"
				target="_blank"
				rel="external noopener noreferrer"
				>{m.platform_settings_product_analytics_update_checks_link()}</a
			>{m.platform_settings_product_analytics_update_checks_suffix()}
		</p>
	</div>
</div>
