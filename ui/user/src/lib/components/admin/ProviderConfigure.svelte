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

	const isAzureOpenAIProvider = $derived(provider && provider.id === 'azure-openai-model-provider');

	const collection = $derived.by(() => {
		if (isAzureOpenAIProvider)
			return {
				title: 'Authentication Method',
				items: [
					{
						id: 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY',
						name: 'API Key'
					},
					{
						id: 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT',
						name: 'Microsoft Entra'
					}
				]
			};
		return undefined;
	});
	let defaultSelectedCollection = $derived.by(() => {
		if (isAzureOpenAIProvider) {
			if (values?.['OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY']) {
				return 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY';
			}

			const hasMicrosoftEntraValues = [
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP'
			].some((param) => Boolean(values?.[param]));

			if (hasMicrosoftEntraValues) {
				return 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT';
			}

			return undefined;
		}
		return undefined;
	});
	let selectedCollection: string | undefined = $derived(
		defaultSelectedCollection ??
			(isAzureOpenAIProvider ? 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY' : undefined)
	);

	const requiredConfigurationParameters = $derived.by(() => {
		if (isAzureOpenAIProvider) {
			const allParams = [
				...(provider?.requiredConfigurationParameters ?? []),
				...(provider?.optionalConfigurationParameters ?? [])
			];

			const asObject = allParams.reduce(
				(acc, val) => {
					acc[val.name] = val;
					return acc;
				},
				{} as Record<string, ProviderParameter>
			);

			if (selectedCollection === 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY') {
				const requiredParamsIds = [
					'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_KEY',
					'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT',
					'OBOT_AZURE_OPENAI_MODEL_PROVIDER_DEPLOYMENTS'
				];
				return requiredParamsIds.map((id) => asObject?.[id]).filter(Boolean) as ProviderParameter[];
			}

			const requiredParamsIds = [
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID',
				'OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP'
			];

			return requiredParamsIds.map((id) => asObject?.[id]).filter(Boolean) as ProviderParameter[];
		}

		return (provider?.requiredConfigurationParameters?.filter((p) => !p.hidden) ??
			[]) as ProviderParameter[];
	});

	const optionalConfigurationParameters = $derived.by(() => {
		const optionalParams = (provider?.optionalConfigurationParameters?.filter((p) => !p.hidden) ??
			[]) as ProviderParameter[];

		if (isAzureOpenAIProvider) {
			// Only show the API version parameter as optional

			const allParams = [
				...(provider?.requiredConfigurationParameters ?? []),
				...(provider?.optionalConfigurationParameters ?? [])
			];

			return allParams.filter(
				(param) => param.name === 'OBOT_AZURE_OPENAI_MODEL_PROVIDER_API_VERSION'
			) as ProviderParameter[];
		}

		return optionalParams;
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

	function reset() {
		showRequired = false;
		form = {};

		if (isAzureOpenAIProvider) {
			// Check if the selected collection is the same as the default one
			if (defaultSelectedCollection === selectedCollection) {
				// Reset to initial values
				onOpen();
				return;
			} else {
				// Clear form values
				for (const param of requiredConfigurationParameters ?? []) {
					form[param.name] = '';
				}

				for (const param of optionalConfigurationParameters ?? []) {
					form[param.name] = '';
				}
			}
		} else {
			onOpen();
		}
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

		if (requiredFieldsNotFilled.length > 0) {
			showRequired = true;
			return;
		}

		const allParams = [...requiredConfigurationParameters, ...optionalConfigurationParameters];

		// Dynamically remove non necessary parameters for Azure OpenAI provider
		// For Azure OpenAI the server does not expect both authentication methods to be sent
		const processedForm = allParams
			.filter((param) => form[param.name].trim())
			.reduce(
				(acc, param) => {
					if (param.multiline) {
						// Convert multiline values to single line with literal \n
						acc[param.name] = form[param.name].replace(/\n/g, '\\n');
					} else {
						acc[param.name] = form[param.name];
					}

					return acc;
				},
				{} as Record<string, string>
			);

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

			{#if requiredConfigurationParameters.length > 0}
				<div class="flex flex-col gap-4">
					{#if collection}
						<div class="mb-4 flex flex-col gap-2">
							<h4 class="text-lg font-semibold">{collection.title}</h4>
							<ul class="flex gap-2">
								{#each collection.items as item, i (i)}
									{@const isSelected = selectedCollection === item.id}
									<li>
										<button
											class={twMerge(
												'bg-surface1 hover:bg-surface2 text-gray rounded-md px-4 py-2 text-sm font-medium transition-all duration-200',
												isSelected &&
													'bg-primary hover:bg-primary/90 active:bg-primary text-white shadow-sm'
											)}
											type="button"
											onclick={() => {
												selectedCollection = item.id;
												// Reset form values
												reset();
											}}>{item.name}</button
										>
									</li>
								{/each}
							</ul>
						</div>
					{/if}

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
											class={['text-input-filled', error && 'error'].filter(Boolean).join(' ')}
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
									<label for={parameter.name}>{parameter.friendlyName}</label>
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
