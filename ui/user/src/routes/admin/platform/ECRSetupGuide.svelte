<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import { m } from '$lib/i18n';

	interface Props {
		effectiveIssuerURL: string;
		effectiveAudience: string;
		trustPolicyJSON: string;
		ecrPolicyJSON: string;
	}

	let { effectiveIssuerURL, effectiveAudience, trustPolicyJSON, ecrPolicyJSON }: Props = $props();
</script>

<div class="flex flex-col gap-1">
	<h3 class="text-base font-semibold">{m.platform_settings_registry_guide_title()}</h3>
	<p class="text-muted-content text-sm">
		{m.platform_settings_registry_guide_description()}
	</p>
</div>

<section class="flex flex-col gap-5">
	<div class="divide-base-300 dark:divide-base-400 flex flex-col divide-y">
		<div class="pb-5">
			{@render setupStep(
				'1',
				m.platform_settings_registry_step1_title(),
				m.platform_settings_registry_step1_description()
			)}
			<div class="mt-4 grid gap-x-6 gap-y-3 pl-9 lg:grid-cols-2">
				{@render setupValue(m.platform_settings_registry_issuer_url(), effectiveIssuerURL)}
				{@render setupValue(m.platform_settings_image_pull_secrets_audience(), effectiveAudience)}
			</div>
		</div>

		<div class="py-5">
			{@render setupStep(
				'2',
				m.platform_settings_registry_step2_title(),
				m.platform_settings_registry_step2_description()
			)}
			<div class="mt-4 pl-9">
				{@render policyBlock(m.platform_settings_registry_trust_policy(), trustPolicyJSON)}
			</div>
		</div>

		<div class="py-5">
			{@render setupStep(
				'3',
				m.platform_settings_registry_step3_title(),
				m.platform_settings_registry_step3_description()
			)}
			<div class="mt-4 pl-9">
				{@render policyBlock(m.platform_settings_registry_iam_policy(), ecrPolicyJSON)}
			</div>
		</div>

		<div class="pt-5">
			{@render setupStep(
				'4',
				m.platform_settings_registry_step4_title(),
				m.platform_settings_registry_step4_description()
			)}
		</div>
	</div>
</section>

{#snippet setupStep(number: string, title: string, description: string)}
	<div class="flex gap-3">
		<div
			class="bg-base-200 text-muted-content flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
		>
			{number}
		</div>
		<div class="min-w-0">
			<h4 class="text-sm font-semibold">{title}</h4>
			<p class="text-muted-content text-sm">{description}</p>
		</div>
	</div>
{/snippet}

{#snippet setupValue(label: string, value?: string)}
	<div class="min-w-0">
		<div class="mb-1 flex items-center gap-2">
			<span class="text-muted-content text-xs font-medium">{label}</span>
			{#if value}
				<CopyButton text={value} />
			{/if}
		</div>
		<div class="text-base-content break-all font-mono text-xs">
			{value || '-'}
		</div>
	</div>
{/snippet}

{#snippet policyBlock(title: string, value?: string)}
	<div>
		<div class="mb-3 flex items-center justify-between gap-2">
			<h4 class="text-sm font-semibold">{title}</h4>
			<CopyButton showTextLeft text={value} />
		</div>
		<pre
			class="default-scrollbar-thin dark:bg-base-400 bg-base-200 max-h-80 overflow-auto rounded-md p-3 text-xs">{value ||
				'-'}</pre>
	</div>
{/snippet}
