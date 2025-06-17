<script lang="ts">
	import type { BaseProvider } from '$lib/services/admin/types';
	import { darkMode, profile } from '$lib/stores';
	import { AlertCircle, LoaderCircle } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import SensitiveInput from '../SensitiveInput.svelte';
	import type { Snippet } from 'svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';

	interface Props {
		provider?: BaseProvider;
		onConfigure: (form: Record<string, string>) => Promise<void>;
		note?: Snippet;
		error?: string;
		values?: Record<string, string>;
		loading?: boolean;
	}

	const { provider, onConfigure, note, values, error, loading }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let form = $state<Record<string, string>>({});

	function onOpen() {
		if (provider) {
			for (const param of provider.requiredConfigurationParameters ?? []) {
				form[param.name] = values?.[param.name] ? values?.[param.name] : '';
			}
			for (const param of provider.optionalConfigurationParameters ?? []) {
				form[param.name] = values?.[param.name] ? values?.[param.name] : '';
			}
		}
	}

	function onClose() {
		form = {};
	}

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function configure() {
		onConfigure(form);
	}
</script>

<ResponsiveDialog bind:this={dialog} {onClose} {onOpen}>
	{#snippet titleContent()}
		{#if darkMode.isDark}
			{@const url = provider?.iconDark ?? provider?.icon}
			<img
				src={url}
				alt={provider?.name}
				class={twMerge('size-9 rounded-md p-1', !provider?.iconDark && 'bg-gray-600')}
			/>
		{:else}
			<img src={provider?.icon} alt={provider?.name} class="bg-surface1 size-9 rounded-md p-1" />
		{/if}
		Set Up {provider?.name}
	{/snippet}
	{#if provider}
		<form class="flex flex-col gap-4" onsubmit={configure}>
			<input
				type="text"
				autocomplete="email"
				name="email"
				value={profile.current.email}
				class="hidden"
			/>
			{#if error}
				<div class="notification-error flex items-center gap-2">
					<AlertCircle class="size-6 text-red-500" />
					<p class="flex flex-col text-sm font-light">
						<span class="font-semibold">An error occurred!</span>
						<span>
							Your configuration could not be saved because it failed validation: <b
								class="font-semibold">{error}</b
							>
						</span>
					</p>
				</div>
			{/if}
			{#if note}
				{@render note()}
			{/if}
			{#if provider.requiredConfigurationParameters && provider.requiredConfigurationParameters.length > 0}
				<div class="flex flex-col gap-4">
					<h4 class="text-lg font-semibold">Required Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each provider.requiredConfigurationParameters as parameter}
							{#if parameter.name in form}
								<li class="flex flex-col gap-1">
									<label for={parameter.name}>{parameter.friendlyName}</label>
									{#if parameter.sensitive}
										<SensitiveInput name={parameter.name} bind:value={form[parameter.name]} />
									{:else}
										<input
											type="text"
											id={parameter.name}
											class="text-input-filled"
											bind:value={form[parameter.name]}
										/>
									{/if}
								</li>
							{/if}
						{/each}
					</ul>
				</div>
			{/if}
			{#if provider.optionalConfigurationParameters && provider.optionalConfigurationParameters.length > 0}
				<div class="flex flex-col gap-2">
					<h4 class="text-lg font-semibold">Optional Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each provider.optionalConfigurationParameters as parameter}
							{#if parameter.name in form}
								<li class="flex flex-col gap-1">
									<label for={parameter.name}>{parameter.friendlyName}</label>
									<input
										type="text"
										id={parameter.name}
										bind:value={form[parameter.name]}
										class="text-input-filled"
									/>
								</li>
							{/if}
						{/each}
					</ul>
				</div>
			{/if}
		</form>
		<div class="mt-4 flex justify-end gap-2">
			<button class="button-primary" onclick={() => configure()} disabled={loading}>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Confirm
				{/if}
			</button>
		</div>
	{/if}
</ResponsiveDialog>
