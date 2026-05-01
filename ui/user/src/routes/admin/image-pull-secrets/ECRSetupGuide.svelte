<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';

	interface Props {
		effectiveIssuerURL: string;
		effectiveAudience: string;
		trustPolicyJSON: string;
		ecrPolicyJSON: string;
	}

	let { effectiveIssuerURL, effectiveAudience, trustPolicyJSON, ecrPolicyJSON }: Props = $props();
</script>

<section class="paper gap-5">
	<div class="flex flex-col gap-1">
		<h3 class="text-on-surface1 text-base font-semibold">AWS Setup Guide</h3>
		<p class="text-on-surface1 text-sm">
			Configure AWS to trust Obot's service account, then save the role ARN above and refresh the
			generated pull secret.
		</p>
	</div>

	<div class="divide-surface2 dark:divide-surface3 flex flex-col divide-y">
		<div class="pb-5">
			{@render setupStep(
				'1',
				'Create an IAM OIDC provider',
				'In AWS IAM, create or reuse an OpenID Connect provider for this Kubernetes service account issuer.'
			)}
			<div class="mt-4 grid gap-x-6 gap-y-3 pl-9 lg:grid-cols-2">
				{@render setupValue('Issuer URL', effectiveIssuerURL)}
				{@render setupValue('Audience', effectiveAudience)}
			</div>
		</div>

		<div class="py-5">
			{@render setupStep(
				'2',
				'Create the IAM role trust policy',
				'Create an IAM role with this trust policy so Obot can assume it with web identity.'
			)}
			<div class="mt-4 pl-9">
				{@render policyBlock('Trust Policy', trustPolicyJSON)}
			</div>
		</div>

		<div class="py-5">
			{@render setupStep(
				'3',
				'Attach ECR pull permissions',
				'Attach this policy to the IAM role, or use an equivalent policy scoped to your repositories.'
			)}
			<div class="mt-4 pl-9">
				{@render policyBlock('ECR IAM Policy', ecrPolicyJSON)}
			</div>
		</div>

		<div class="pt-5">
			{@render setupStep(
				'4',
				'Save and refresh in Obot',
				'Paste the role ARN into the form, save the credential, then run Refresh Now to write the Kubernetes image pull secret.'
			)}
		</div>
	</div>
</section>

{#snippet setupStep(number: string, title: string, description: string)}
	<div class="flex gap-3">
		<div
			class="bg-surface1 text-on-surface1 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
		>
			{number}
		</div>
		<div class="min-w-0">
			<h4 class="text-sm font-semibold">{title}</h4>
			<p class="text-on-surface1 text-sm">{description}</p>
		</div>
	</div>
{/snippet}

{#snippet setupValue(label: string, value?: string)}
	<div class="min-w-0">
		<div class="mb-1 flex items-center gap-2">
			<span class="text-on-surface1 text-xs font-medium">{label}</span>
			{#if value}
				<CopyButton text={value} />
			{/if}
		</div>
		<div class="text-on-surface1 break-all font-mono text-xs">
			{value || '-'}
		</div>
	</div>
{/snippet}

{#snippet policyBlock(title: string, value?: string)}
	<div>
		<div class="mb-3 flex items-center justify-between gap-2">
			<h4 class="text-sm font-semibold">{title}</h4>
			<CopyButton text={value} />
		</div>
		<pre
			class="default-scrollbar-thin dark:border-surface3 bg-surface1 dark:bg-background max-h-80 overflow-auto rounded-md border border-transparent p-3 text-xs">{value ||
				'-'}</pre>
	</div>
{/snippet}
