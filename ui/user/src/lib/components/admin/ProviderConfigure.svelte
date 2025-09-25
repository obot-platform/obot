<script lang="ts">
	import type { BaseProvider } from '$lib/services/admin/types';
	import { darkMode, profile } from '$lib/stores';
	import { AlertCircle, LoaderCircle } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import SensitiveInput from '../SensitiveInput.svelte';
	import type { Snippet } from 'svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { MultiValueInput } from '$lib/components/ui/multi-value-input';

	interface Props {
		provider?: BaseProvider;
		onConfigure: (form: Record<string, string>) => Promise<void>;
		note?: Snippet;
		error?: string;
		values?: Record<string, string>;
		loading?: boolean;
		readonly?: boolean;
	}

	const { provider, onConfigure, note, values, error, loading, readonly }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let form = $state<Record<string, string>>({});
	let showRequired = $state(false);

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

	const multipValuesInputs = new Set([
		'OBOT_GITHUB_AUTH_PROVIDER_ALLOW_USERS',
		'OBOT_GITHUB_AUTH_PROVIDER_TEAMS',
		'OBOT_GITHUB_AUTH_PROVIDER_REPO',
		'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS'
	]);
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
			onsubmit={readonly ? undefined : configure}
		>
			<input
				type="text"
				autocomplete="email"
				name="email"
				value={profile.current.email}
				class="hidden"
				disabled={readonly}
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
			{#if requiredConfigurationParameters.length > 0}
				<div class="flex flex-col gap-4">
					<h4 class="text-lg font-semibold">Required Configuration</h4>
					<ul class="flex flex-col gap-4">
						{#each requiredConfigurationParameters as parameter (parameter.name)}
							{#if parameter.name in form}
								{@const error = !form[parameter.name].length && showRequired}
								<li class="flex flex-col gap-1">
									<label for={parameter.name} class:text-red-500={error}
										>{parameter.friendlyName}</label
									>
									{#if parameter.description}
										<span class="text-gray text-xs">{parameter.description}</span>
									{/if}
									{#if parameter.sensitive}
										<SensitiveInput
											{error}
											name={parameter.name}
											bind:value={form[parameter.name]}
											disabled={readonly}
										/>
									{:else if multipValuesInputs.has(parameter.name)}
										<MultiValueInput
											bind:value={form[parameter.name]}
											id={parameter.name}
											labels={parameter.name === 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS'
												? { '*': 'All domains' }
												: {}}
											class="text-input-filled"
											placeholder={`Hit "Enter" to insert`.toString()}
											disabled={readonly}
										/>
									{:else}
										<input
											type="text"
											id={parameter.name}
											bind:value={form[parameter.name]}
											class:error
											class="text-input-filled"
											disabled={readonly}
										/>
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
								<li class="flex flex-col gap-1">
									<label for={parameter.name}>{parameter.friendlyName}</label>
									<span class="text-gray text-xs">{parameter.description}</span>
									{#if multipValuesInputs.has(parameter.name)}
										<MultiValueInput
											bind:value={form[parameter.name]}
											id={parameter.name}
											class="text-input-filled"
											placeholder={`Hit "Enter" to insert`.toString()}
											disabled={readonly}
										/>
									{:else}
										<input
											type="text"
											id={parameter.name}
											bind:value={form[parameter.name]}
											class="text-input-filled"
											disabled={readonly}
										/>
									{/if}
								</li>
							{/if}
						{/each}
					</ul>
				</div>
			{/if}
		</form>
		{#if !readonly}
			<div class="mt-4 flex justify-end gap-2 p-4 pt-0">
				<button class="button-primary" type="button" onclick={() => configure()} disabled={loading}>
					{#if loading}
						<LoaderCircle class="size-4 animate-spin" />
					{:else}
						Confirm
					{/if}
				</button>
			</div>
		{/if}
	{/if}
</ResponsiveDialog>
