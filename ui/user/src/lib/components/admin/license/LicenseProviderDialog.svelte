<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import type { BaseProvider } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { clearUrlParams } from '$lib/url';
	import Confirm from '../../Confirm.svelte';
	import { CircleAlert, LoaderCircle, TriangleAlert } from '@lucide/svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider?: BaseProvider;
		licenseKey?: string;
		allowSignup?: boolean;
		onSubmit?: (response: unknown) => Promise<void>;
		endpoint?: (data: { name: string; email: string; company?: string }) => Promise<unknown>;
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

	let saving = $state(false);
	let error = $state('');
	let formData = $state({
		name: '',
		email: '',
		company: ''
	});

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (!endpoint) return;
		if (saving) return;

		saving = true;
		error = '';
		try {
			const response = await endpoint({
				name: formData.name.trim(),
				email: formData.email.trim(),
				company: formData.company.trim()
			});
			console.log('response', response);
			await onSubmit?.(response);
			console.log('onSubmit', onSubmit);
		} catch (err) {
			error = parseErrorContent(err).message || 'Failed to obtain an Obot Community license.';
		} finally {
			saving = false;
			clearUrlParams(['provider']);
		}
	}
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
			<p>
				{#if allowSignup}
					{@render signUpForm()}
				{:else if provider?.configured}
					Your license for or access to {provider.name} is invalid. Please contact support at
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a> to renew your license.
				{:else}
					A valid license is required to use {provider.name}. Please contact support at
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a> for more information or to
					upgrade to Obot Enterprise.
				{/if}
			</p>
		{/if}
	{/snippet}
</Confirm>

{#snippet signUpForm()}
	<form
		class="flex flex-col gap-4 text-start p-4 border border-base-300 rounded-md"
		onsubmit={handleSubmit}
	>
		<div class="flex flex-col gap-1">
			<h4 class="font-semibold text-lg text-center">Get Access Now!</h4>
			<p class="text-muted-content text-sm font-light text-center">
				{signUpMessage || 'Register your email below to gain access to additional features!'}
			</p>
		</div>
		<div class="flex flex-col gap-4">
			<label class="flex flex-col gap-1 text-sm font-light" for="community-name">
				Name
				<input
					id="community-name"
					class="text-input-filled"
					name="name"
					type="text"
					autocomplete="name"
					bind:value={formData.name}
					required
				/>
			</label>

			<label class="flex flex-col gap-1 text-sm font-light" for="community-email">
				Email
				<input
					id="community-email"
					class="text-input-filled"
					name="email"
					type="email"
					pattern="[^\s@]+@[^\s@.]+(?:\.[^\s@.]+)+"
					title="Enter an email address with a valid domain, such as name@example.com."
					autocomplete="email"
					bind:value={formData.email}
					required
				/>
			</label>

			<label class="flex flex-col gap-1 text-sm font-light" for="community-company">
				Company <span class="text-muted-content text-xs">(optional)</span>
				<input
					id="community-company"
					class="text-input-filled"
					name="company"
					type="text"
					autocomplete="organization"
					bind:value={formData.company}
				/>
			</label>
		</div>

		{#if error}
			<div in:slide={{ duration: 150, axis: 'y' }} class="alert alert-error alert-soft">
				{error}
			</div>
		{/if}

		<button class="btn btn-primary w-full my-2" type="submit" disabled={saving}>
			{#if saving}
				<LoaderCircle class="size-4 animate-spin" />
			{/if}
			{saving ? 'Registering...' : 'Register'}
		</button>
	</form>
{/snippet}
