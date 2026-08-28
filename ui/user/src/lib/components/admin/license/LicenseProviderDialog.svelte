<script lang="ts">
	import type { BaseProvider } from '$lib/services';
	import type { CommunityLicenseEnrollment } from '$lib/services/admin/types';
	import { darkMode } from '$lib/stores';
	import { clearUrlParams } from '$lib/url';
	import Confirm from '../../Confirm.svelte';
	import CommunitySignUpForm from './CommunitySignUpForm.svelte';
	import { CircleAlert, TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider?: BaseProvider;
		licenseKey?: string;
		allowSignup?: boolean;
		onSubmit?: (response: unknown) => Promise<void> | void;
		endpoint?: (data: CommunityLicenseEnrollment) => Promise<unknown>;
		signUpMessage?: string;
	}

	let {
		provider = $bindable(),
		licenseKey,
		allowSignup,
		onSubmit,
		endpoint,
		signUpMessage
	}: Props = $props();
</script>

<Confirm
	hideCancelButton
	show={!!provider}
	oncancel={() => {
		provider = undefined;
		clearUrlParams(['provider']);
	}}
	cancelText="Close"
>
	{#snippet titleContent()}
		{#if provider}
			<div class="flex items-center gap-2">
				{#if darkMode.isDark}
					{@const url = provider.iconDark ?? provider.icon}
					<img
						src={url}
						alt={provider.name}
						class={twMerge('size-6 shrink-0 rounded-md p-1', !provider.iconDark && 'bg-base-400')}
					/>
				{:else}
					<img src={provider.icon} alt={provider.name} class="size-6 shrink-0 rounded-md p-1" />
				{/if}
				<h3 class="text-lg font-semibold">{provider.name}</h3>
			</div>
		{/if}
	{/snippet}
	{#snippet msgContent()}
		{#if !allowSignup}
			<div class="flex items-center gap-2">
				{#if provider?.configured}
					<TriangleAlert class="size-4 text-warning" />
					<h4 class="font-semibold text-base">License {licenseKey ? 'Invalid' : 'Missing'}</h4>
				{:else}
					<CircleAlert class="size-4 text-muted-content" />
					<h4 class="font-semibold text-base">License Required</h4>
				{/if}
			</div>
		{/if}
	{/snippet}
	{#snippet note()}
		{#if provider}
			{#if allowSignup}
				<CommunitySignUpForm {endpoint} {onSubmit} {signUpMessage} />
			{:else if provider?.configured}
				<p>
					Your license for or access to {provider.name} is invalid. Please contact support at
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a> to renew your license.
				</p>
			{:else}
				<p>
					A valid license is required to use {provider.name}. Please contact support at
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a> for more information or to
					upgrade to Obot Enterprise.
				</p>
			{/if}
		{/if}
	{/snippet}
</Confirm>
