<script lang="ts">
	import { parseErrorContent } from '$lib/errors';
	import type { CommunityLicenseEnrollment } from '$lib/services/admin/types';
	import { clearUrlParams } from '$lib/url';
	import { LoaderCircle } from '@lucide/svelte';
	import { slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		endpoint?: (data: CommunityLicenseEnrollment) => Promise<unknown>;
		onSubmit?: (response: unknown) => Promise<void> | void;
		signUpMessage?: string;
		showHeader?: boolean;
		compact?: boolean;
		idPrefix?: string;
		disabled?: boolean;
		class?: string;
	}

	let {
		endpoint,
		onSubmit,
		signUpMessage,
		showHeader = true,
		compact = false,
		idPrefix = 'community',
		disabled = false,
		class: className
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
		if (saving || disabled) return;

		saving = true;
		error = '';
		try {
			const company = formData.company ? formData.company.trim() : '';
			const response = await endpoint({
				name: formData.name.trim(),
				email: formData.email.trim(),
				company: company.length > 0 ? company : undefined
			});
			await onSubmit?.(response);
		} catch (err) {
			error = parseErrorContent(err).message || 'Error occurred during registration.';
		} finally {
			saving = false;
			if (new URL(window.location.href).searchParams.has('provider')) {
				clearUrlParams(['provider']);
			}
		}
	}
</script>

<form
	class={twMerge(
		'flex flex-col gap-3 text-start',
		showHeader && 'rounded-md border border-base-300 p-4 gap-4',
		compact && '@xl:flex-row @xl:flex-wrap @xl:items-end @xl:gap-3',
		className
	)}
	onsubmit={handleSubmit}
>
	{#if showHeader}
		<div class="flex flex-col gap-1">
			<h4 class="text-center text-lg font-semibold">Get Access Now!</h4>
			<p class="text-center text-sm font-light text-muted-content">
				{signUpMessage || 'Register your email below to gain access to additional features!'}
			</p>
		</div>
	{/if}
	<div
		class={twMerge(
			'flex flex-col gap-3',
			showHeader && 'gap-4',
			compact && '@xl:min-w-0 @xl:flex-1 @xl:flex-row @xl:gap-3'
		)}
	>
		<label
			class={twMerge(
				'flex flex-col gap-1 text-sm font-light',
				compact && '@xl:min-w-28 @xl:flex-1'
			)}
			for={idPrefix + '-name'}
		>
			Name
			<input
				id={idPrefix + '-name'}
				class="text-input-filled"
				name="name"
				type="text"
				autocomplete="name"
				bind:value={formData.name}
				required
				{disabled}
			/>
		</label>

		<label
			class={twMerge(
				'flex flex-col gap-1 text-sm font-light',
				compact && '@xl:min-w-28 @xl:flex-1'
			)}
			for={idPrefix + '-email'}
		>
			Email
			<input
				id={idPrefix + '-email'}
				class="text-input-filled"
				name="email"
				type="email"
				pattern="[^\s@]+@[^\s@.]+(?:\.[^\s@.]+)+"
				title="Enter an email address with a valid domain, such as name@example.com."
				autocomplete="email"
				bind:value={formData.email}
				required
				{disabled}
			/>
		</label>

		<label
			class={twMerge(
				'flex flex-col gap-1 text-sm font-light',
				compact && '@xl:min-w-28 @xl:flex-1'
			)}
			for={idPrefix + '-company'}
		>
			Company <span class="text-xs text-muted-content">(optional)</span>
			<input
				id="{idPrefix}-company"
				class="text-input-filled"
				name="company"
				type="text"
				autocomplete="organization"
				bind:value={formData.company}
				{disabled}
			/>
		</label>
	</div>

	{#if error}
		<div
			in:slide={{ duration: 150, axis: 'y' }}
			class={twMerge('alert alert-error alert-soft', compact && '@xl:w-full')}
		>
			{error}
		</div>
	{/if}

	<button
		class={twMerge(
			'btn btn-primary w-full',
			showHeader && 'my-2',
			compact && '@xl:my-0 @xl:w-auto @xl:shrink-0'
		)}
		type="submit"
		disabled={saving || disabled}
	>
		{#if saving}
			<LoaderCircle class="size-4 animate-spin" />
		{/if}
		{saving ? 'Registering...' : 'Register'}
	</button>
</form>
