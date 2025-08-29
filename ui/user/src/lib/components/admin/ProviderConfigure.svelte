<script lang="ts">
	import type { BaseProvider, ValidationError } from '$lib/services/admin/types';
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
		error?: ValidationError | string;
		values?: Record<string, string>;
		loading?: boolean;
	}

	const { provider, onConfigure, note, values, error, loading }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let form = $state<Record<string, string>>({});
	let showRequired = $state(false);

	// Helper function to get field error by parameter name
	function getFieldError(parameterName: string): string | undefined {
		// Check if error is a ValidationError object with FieldError array
		if (!error || typeof error === 'string' || !error.error || !Array.isArray(error.error))
			return undefined;

		// Find matching field error by envVar
		const fieldError = error.error.find((err) => err.envVar === parameterName);
		return fieldError?.message;
	}

	// Helper to get the general error message
	function getGeneralError(): string | undefined {
		return typeof error === 'string' ? error : undefined;
	}

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
		showRequired = false;
		const requiredFields =
			provider?.requiredConfigurationParameters?.filter((p) => !p.hidden) ?? [];
		const requiredFieldsNotFilled = requiredFields.filter((p) => !form[p.name].length);
		if (requiredFieldsNotFilled.length > 0) {
			showRequired = true;
			return;
		}
		onConfigure(form);
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	{onClose}
	{onOpen}
	class="p-0"
	classes={{ header: 'p-4 pb-0' }}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2 pb-0">
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
		</div>
	{/snippet}
	{#if provider}
		{@const requiredConfigurationParameters =
			provider.requiredConfigurationParameters?.filter((p) => !p.hidden) ?? []}
		{@const optionalConfigurationParameters =
			provider.optionalConfigurationParameters?.filter((p) => !p.hidden) ?? []}
		<form
			class="default-scrollbar-thin flex max-h-[70vh] flex-col gap-4 overflow-y-auto p-4 pt-0"
			onsubmit={configure}
		>
			<input
				type="text"
				autocomplete="email"
				name="email"
				value={profile.current.email}
				class="hidden"
			/>
			{#if getGeneralError()}
				<div class="notification-error flex items-center gap-2">
					<AlertCircle class="size-6 text-red-500" />
					<p class="flex flex-col text-sm font-light">
						<span class="font-semibold">An error occurred!</span>
						<span>
							Your configuration could not be saved because it failed validation: <b
								class="font-semibold">{getGeneralError()}</b
							>
						</span>
					</p>
				</div>
			{/if}
			{#if note}
				{@render note()}
			{/if}
			{#if requiredConfigurationParameters.length > 0}
				<div class="flex flex-col gap-4">
					<h4 class="text-lg font-semibold">Required Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each requiredConfigurationParameters as parameter (parameter.name)}
							{#if parameter.name in form}
								{@const fieldError = getFieldError(parameter.name)}
								{@const requiredError = !form[parameter.name].length && showRequired}
								{@const hasError = fieldError || requiredError}
								<li class="flex flex-col gap-1">
									<label for={parameter.name} class:text-red-500={hasError}
										>{parameter.friendlyName}</label
									>
									{#if parameter.sensitive}
										<SensitiveInput
											error={!!hasError}
											name={parameter.name}
											bind:value={form[parameter.name]}
										/>
									{:else}
										<input
											type="text"
											id={parameter.name}
											class="text-input-filled"
											class:error={hasError}
											bind:value={form[parameter.name]}
										/>
									{/if}
									{#if fieldError}
										<span class="text-sm text-red-500">{fieldError}</span>
									{/if}
								</li>
							{/if}
						{/each}
					</ul>
				</div>
			{/if}
			{#if optionalConfigurationParameters.length > 0}
				<div class="flex flex-col gap-2">
					<h4 class="text-lg font-semibold">Optional Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each optionalConfigurationParameters as parameter (parameter.name)}
							{#if parameter.name in form}
								{@const fieldError = getFieldError(parameter.name)}
								<li class="flex flex-col gap-1">
									<label for={parameter.name} class:text-red-500={fieldError}
										>{parameter.friendlyName}</label
									>
									<input
										type="text"
										id={parameter.name}
										bind:value={form[parameter.name]}
										class="text-input-filled"
										class:error={fieldError}
									/>
									{#if fieldError}
										<span class="text-sm text-red-500">{fieldError}</span>
									{/if}
								</li>
							{/if}
						{/each}
					</ul>
				</div>
			{/if}
		</form>
		<div class="mt-4 flex justify-end gap-2 p-4 pt-0">
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
