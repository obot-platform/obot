<script lang="ts">
	import type { BaseProvider, ProviderParameter } from '$lib/services/admin/types';
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

	const isAzureOpernAIProvider = $derived(
		provider && provider.id === 'azure-openai-model-provider'
	);
	const requiredConfigurationPairs = $derived.by(() => {
		if (!isAzureOpernAIProvider) return [];

		const endpoint = provider?.requiredConfigurationParameters?.find(
			(p) => p.name === 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT'
		);
		const apiKey = provider?.optionalConfigurationParameters?.find(
			(p) => p.name === 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY'
		);

		type Pair = NonNullable<typeof apiKey>;

		return [[endpoint, apiKey].filter(Boolean) as Pair[]];
	});

	const filterOutParams = $derived.by(() => {
		if (isAzureOpernAIProvider)
			return [
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT'
			];
		return [];
	});

	const requiredConfigurationParameters = $derived(
		provider?.requiredConfigurationParameters?.filter(
			(p) => !p.hidden && !filterOutParams.includes(p.name)
		) ?? []
	);
	const optionalConfigurationParameters = $derived(
		provider?.optionalConfigurationParameters?.filter(
			(p) => !p.hidden && !filterOutParams.includes(p.name)
		) ?? []
	);

	const selectedParameterPair: Record<string, ProviderParameter | undefined> = $state({});
	const defaultSelectedParameterPair = $derived.by(() => {
		return requiredConfigurationPairs.reduce(
			(acc, pair, i) => {
				acc[i] = pair[0];
				return acc;
			},
			{} as Record<string, ProviderParameter | undefined>
		);
	});

	const readonlySelectedParameterPair = $derived({
		...defaultSelectedParameterPair,
		...selectedParameterPair
	});

	function onOpen() {
		// Reset state on each open
		form = {};
		showRequired = false;

		if (provider) {
			for (const param of provider.requiredConfigurationParameters ?? []) {
				let value = values?.[param.name] ? values?.[param.name] : '';
				// Convert literal \n to actual newlines for multiline fields
				if (param.multiline && value) {
					value = value.replace(/\\n/g, '\n');
				}
				form[param.name] = value;
			}
			for (const param of provider.optionalConfigurationParameters ?? []) {
				let value = values?.[param.name] ? values?.[param.name] : '';
				// Convert literal \n to actual newlines for multiline fields
				if (param.multiline && value) {
					value = value.replace(/\\n/g, '\n');
				}
				form[param.name] = value;
			}
		}
	}

	function onClose() {
		form = {};
		showRequired = false;
	}

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	async function configure() {
		showRequired = false;

		const requiredFieldsNotFilled = requiredConfigurationParameters.filter(
			(p) => !form[p.name].length
		);

		// Check required pairs fields; at least one in each pair must be filled; the selected one should be filled
		for (let index = 0; index < requiredConfigurationPairs.length; index++) {
			const selectedParameter = readonlySelectedParameterPair[index + ''];

			if (selectedParameter && !form[selectedParameter.name].length) {
				requiredFieldsNotFilled.push(selectedParameter);
			}
		}

		if (requiredFieldsNotFilled.length > 0) {
			showRequired = true;
			return;
		}

		// Convert multiline values to single line with literal \n
		const processedForm = { ...form };

		const selectedPairs = Object.values(readonlySelectedParameterPair).filter(
			Boolean
		) as ProviderParameter[];
		const allParams = [
			...selectedPairs,
			...requiredConfigurationParameters,
			...optionalConfigurationParameters
		];

		for (const param of allParams) {
			if (param.multiline && processedForm[param.name]) {
				processedForm[param.name] = processedForm[param.name].replace(/\n/g, '\\n');
			}
		}

		onConfigure(processedForm);
	}

	const multipValuesInputs = new Set([
		'OBOT_GITHUB_AUTH_PROVIDER_ALLOW_USERS',
		'OBOT_GITHUB_AUTH_PROVIDER_TEAMS',
		'OBOT_GITHUB_AUTH_PROVIDER_REPO',
		'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS',
		'OBOT_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENTS'
	]);
</script>

<ResponsiveDialog
	bind:this={dialog}
	{onClose}
	{onOpen}
	classes={{ header: 'p-4 pb-0', content: 'p-0' }}
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

			{#if requiredConfigurationParameters.length > 0 || requiredConfigurationPairs.length > 0}
				<div class="flex flex-col gap-4">
					<h4 class="text-lg font-semibold">Required Configuration</h4>
					{#if requiredConfigurationPairs.length > 0}
						<ul class="flex flex-col gap-4">
							{#each requiredConfigurationPairs as pair, i (i)}
								{@const selectedParameter = readonlySelectedParameterPair[i + '']}
								{@const hasError =
									selectedParameter && !form[selectedParameter.name]?.length && showRequired}

								<li class="flex flex-col gap-2">
									<div class="flex gap-1">
										{#each pair as parameter (parameter.name)}
											{@const isSelected = selectedParameter?.name === parameter.name}
											<button
												class={twMerge(
													'bg-surface1 hover:bg-surface2 text-gray rounded-md px-4 py-2 text-sm font-medium transition-all duration-200',
													isSelected &&
														'bg-primary hover:bg-primary/90 active:bg-primary text-white shadow-sm',
													isSelected &&
														hasError &&
														'bg-red-500 text-white hover:bg-red-600/90 active:bg-red-600'
												)}
												type="button"
												onclick={() => {
													// Clear errors when switching
													showRequired = false;
													selectedParameterPair[i + ''] = parameter;
												}}>{parameter.friendlyName}</button
											>
										{/each}
									</div>

									{#if selectedParameter && typeof form[selectedParameter.name] === 'string'}
										<div class="flex flex-col gap-1">
											{#if selectedParameter.description}
												<span class="text-gray text-xs">{selectedParameter.description}</span>
											{/if}
											{#if selectedParameter.sensitive}
												<SensitiveInput
													error={hasError}
													name={selectedParameter.name}
													bind:value={form[selectedParameter.name]}
													disabled={readonly}
													textarea={selectedParameter.multiline}
													growable={selectedParameter.multiline}
												/>
											{:else if multipValuesInputs.has(selectedParameter.name)}
												<MultiValueInput
													bind:value={form[selectedParameter.name]}
													id={selectedParameter.name}
													labels={selectedParameter.name === 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS'
														? { '*': 'All domains' }
														: {}}
													class="text-input-filled"
													placeholder={`Hit "Enter" to insert`.toString()}
													disabled={readonly}
												/>
											{:else if selectedParameter.multiline}
												<textarea
													id={selectedParameter.name}
													bind:value={form[selectedParameter.name]}
													class:error={hasError}
													class="text-input-filled min-h-[120px] resize-y"
													disabled={readonly}
													rows="5"
												></textarea>
											{:else}
												<input
													type="text"
													id={selectedParameter.name}
													bind:value={form[selectedParameter.name]}
													class:error={hasError}
													class="text-input-filled"
													disabled={readonly}
												/>
											{/if}
										</div>
									{/if}
								</li>
							{/each}
						</ul>
					{/if}

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
											textarea={parameter.multiline}
											growable={parameter.multiline}
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
									{:else if parameter.multiline}
										<textarea
											id={parameter.name}
											bind:value={form[parameter.name]}
											class:error
											class="text-input-filled min-h-[120px] resize-y"
											disabled={readonly}
											rows="5"
										></textarea>
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
									<label for={parameter.name} class:text-red-500={error}
										>{parameter.friendlyName}</label
									>
									{#if parameter.description}
										<span class="text-gray text-xs">{parameter.description}</span>
									{/if}
									{#if parameter.sensitive}
										<SensitiveInput
											name={parameter.name}
											bind:value={form[parameter.name]}
											disabled={readonly}
											textarea={parameter.multiline}
											growable={parameter.multiline}
										/>
									{:else if multipValuesInputs.has(parameter.name)}
										<MultiValueInput
											bind:value={form[parameter.name]}
											id={parameter.name}
											class="text-input-filled"
											placeholder={`Hit "Enter" to insert`.toString()}
											disabled={readonly}
										/>
									{:else if parameter.multiline}
										<textarea
											id={parameter.name}
											bind:value={form[parameter.name]}
											class="text-input-filled min-h-[120px] resize-y"
											disabled={readonly}
											rows="5"
										></textarea>
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
