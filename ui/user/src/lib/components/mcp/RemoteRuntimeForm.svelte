<script lang="ts">
	import type {
		RemoteCatalogConfigAdmin,
		RemoteRuntimeConfigAdmin
	} from '$lib/services/admin/types';
	import { Plus, Trash2, Info, Settings } from 'lucide-svelte';
	import Select from '../Select.svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { fade, slide } from 'svelte/transition';
	import Toggle from '../Toggle.svelte';
	import { twMerge } from 'tailwind-merge';
	import InfoTooltip from '../InfoTooltip.svelte';

	interface Props {
		config: RemoteCatalogConfigAdmin | RemoteRuntimeConfigAdmin;
		readonly?: boolean;
		showRequired?: Record<string, boolean>;
		onFieldChange?: (field: string) => void;
		isNewEntry?: boolean;
		onConfigureOAuth?: () => void;
	}
	let {
		config = $bindable(),
		readonly,
		showRequired,
		onFieldChange,
		isNewEntry,
		onConfigureOAuth
	}: Props = $props();

	// For catalog entries, we show advanced config if hostname, urlTemplate, or headers exist
	// For servers, we always show the URL field (no advanced toggle needed)
	let showAdvanced = $state(
		Boolean(
			(config as RemoteCatalogConfigAdmin).hostname ||
				(config as RemoteCatalogConfigAdmin).urlTemplate ||
				(config.headers && config.headers.length > 0) ||
				(config as RemoteCatalogConfigAdmin).staticOAuthRequired
		)
	);

	let selectedType = $state<'fixedURL' | 'hostname' | 'urlTemplate'>(
		(config as RemoteCatalogConfigAdmin).urlTemplate &&
			(config as RemoteCatalogConfigAdmin).urlTemplate!.length > 0
			? 'urlTemplate'
			: (config as RemoteCatalogConfigAdmin).hostname &&
				  (config as RemoteCatalogConfigAdmin).hostname!.length > 0
				? 'hostname'
				: 'fixedURL'
	);
</script>

{#if !showAdvanced}
	{@const remoteConfig = config as RemoteCatalogConfigAdmin}
	<!-- For catalog entries, show simple fixed URL when not in advanced mode -->
	<div
		class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		in:fade={{ duration: 200 }}
	>
		<label
			for="basic-url"
			class={twMerge('w-24 text-sm font-light', showRequired?.fixedURL && 'error')}>URL</label
		>
		<input
			id="basic-url"
			class={twMerge(
				'text-input-filled dark:bg-background flex grow',
				showRequired?.fixedURL && 'error'
			)}
			bind:value={remoteConfig.fixedURL}
			disabled={readonly || showAdvanced}
		/>
	</div>
{/if}

{#if showAdvanced}
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<div class="flex items-center gap-4 {readonly ? 'hidden' : ''}">
				<label for="remote-type" class="flex-shrink-0 text-sm font-light"
					>Restrict connections to:</label
				>
				<Select
					class="bg-surface1 dark:border-surface3 dark:bg-background border border-transparent shadow-inner"
					classes={{
						root: 'flex grow'
					}}
					options={[
						{ label: 'Exact URL', id: 'fixedURL' },
						{ label: 'Hostname', id: 'hostname' },
						{ label: 'URL Template', id: 'urlTemplate' }
					]}
					selected={selectedType}
					onSelect={(option) => {
						const catalogConfig = config as RemoteCatalogConfigAdmin;
						if (option.id === 'fixedURL') {
							catalogConfig.hostname = undefined;
							catalogConfig.urlTemplate = undefined;
							selectedType = 'fixedURL';
							catalogConfig.fixedURL = '';
						} else if (option.id === 'hostname') {
							catalogConfig.fixedURL = undefined;
							catalogConfig.urlTemplate = undefined;
							catalogConfig.hostname = '';
							selectedType = 'hostname';
						} else if (option.id === 'urlTemplate') {
							catalogConfig.fixedURL = undefined;
							catalogConfig.hostname = undefined;
							catalogConfig.urlTemplate = '';
							selectedType = 'urlTemplate';
						}
					}}
				/>
			</div>
			{#if selectedType === 'fixedURL' && typeof (config as RemoteCatalogConfigAdmin).fixedURL !== 'undefined'}
				{@const remoteConfig = config as RemoteCatalogConfigAdmin}
				<div class="flex items-center gap-2">
					<label
						for="remote-url"
						class={twMerge('min-w-18 text-sm font-light', showRequired?.fixedURL && 'error')}
						>Exact URL</label
					>
					<input
						class={twMerge(
							'text-input-filled dark:bg-background flex grow',
							showRequired?.fixedURL && 'error'
						)}
						bind:value={remoteConfig.fixedURL}
						disabled={readonly}
						placeholder="e.g. https://custom.mcpserver.example.com/go/to"
						oninput={() => {
							onFieldChange?.('fixedURL');
						}}
					/>
				</div>
			{:else if selectedType === 'hostname' && typeof (config as RemoteCatalogConfigAdmin).hostname !== 'undefined'}
				{@const remoteConfig = config as RemoteCatalogConfigAdmin}
				<div class="flex items-center gap-2">
					<label
						for="remote-url"
						class={twMerge('min-w-18 text-sm font-light', showRequired?.hostname && 'error')}
						>Hostname</label
					>
					<input
						class={twMerge(
							'text-input-filled dark:bg-background flex grow',
							showRequired?.hostname && 'error'
						)}
						bind:value={remoteConfig.hostname}
						disabled={readonly}
						placeholder="e.g. mycustomdomain"
						oninput={() => {
							onFieldChange?.('hostname');
						}}
					/>
				</div>
			{:else if selectedType === 'urlTemplate' && typeof (config as RemoteCatalogConfigAdmin).urlTemplate !== 'undefined'}
				{@const remoteConfig = config as RemoteCatalogConfigAdmin}
				<div class="flex flex-col gap-4">
					<div class="flex items-center gap-2">
						<label
							for="remote-url-template"
							class={twMerge('min-w-18 text-sm font-light', showRequired?.urlTemplate && 'error')}
							>URL Template</label
						>
						<input
							class={twMerge(
								'text-input-filled dark:bg-background flex grow',
								showRequired?.urlTemplate && 'error'
							)}
							bind:value={remoteConfig.urlTemplate}
							disabled={readonly}
							placeholder={`e.g. https://$${'{API_HOST}'}/api/$${'{VERSION}'}/endpoint`}
							oninput={() => {
								onFieldChange?.('urlTemplate');
							}}
						/>
					</div>

					<!-- Info message about header interpolation -->
					<div class="notification-info p-3 text-sm font-light">
						<div class="flex items-start gap-3">
							<Info class="mt-0.5 size-5 flex-shrink-0" />
							<div class="flex flex-col gap-1">
								<p class="font-semibold">Variable Interpolation</p>
								<p>
									Use <code class="rounded bg-gray-100 px-1 py-0.5 dark:bg-gray-800"
										>${'{VARIABLE_NAME}'}</code
									> syntax in your URL template. Variables can be populated from header values that users
									provide during setup.
								</p>
								<p class="text-xs">
									Example: <code class="rounded bg-gray-100 px-1 py-0.5 text-xs dark:bg-gray-800"
										>https://${'{WORKSPACE_URL}'}/api/2.0/mcp/genie/${'{SPACE_ID}'}</code
									>
								</p>
								<br />
								<p>
									Avoid including variables in your URL template that may contain sensitive
									information, such as API keys. Even when using HTTPS, URLs can be logged or cached
									by browsers, servers, and monitoring systems, potentially exposing confidential
									data. Instead, place sensitive values in HTTP headers (for example, <code
										>Authorization: Bearer &lt;token&gt;</code
									>).
								</p>
							</div>
						</div>
					</div>
				</div>
			{/if}
		</div>
	</div>
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<h4 class="text-sm font-semibold">Headers</h4>
			<p class="text-on-surface1 text-xs font-light">
				{#if selectedType === 'urlTemplate'}
					Header values will be supplied with the URL to configure the MCP server. Their values can
					be supplied by the user during initial setup or as static provided values. Only values
					provided by the user will be used in URL template interpolation.
				{:else}
					Header values will be supplied with the URL to configure the MCP server. Their values can
					be supplied by the user during initial setup or as static provided values.
				{/if}
			</p>
			{#if config.headers}
				{#each config.headers as header, i (i)}
					<div
						class="dark:border-surface3 bg-surface2 flex w-full items-center gap-4 rounded-lg border border-transparent p-4"
					>
						<div class="flex w-full flex-col gap-4">
							<div class="flex w-full flex-col gap-1">
								<label for={`header-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`header-key-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config.headers[i].key}
									placeholder="e.g. CUSTOM_HEADER_KEY"
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`env-type-${i}`} class="text-sm font-light">Value</label>
								<Select
									class="bg-background dark:border-surface3 border border-transparent shadow-none"
									classes={{
										root: 'flex grow'
									}}
									options={[
										{ label: 'Static', id: 'static' },
										{ label: 'User-Supplied', id: 'user_supplied' }
									]}
									selected={config.headers[i].required ? 'user_supplied' : 'static'}
									onSelect={(option) => {
										if (!config.headers?.[i]) return;
										if (option.id === 'user_supplied') {
											config.headers[i].required = true;
										} else {
											config.headers[i].required = false;
											config.headers[i].name = '';
											config.headers[i].description = '';
											config.headers[i].sensitive = false;
										}
										config.headers[i].value = '';
									}}
									id={`env-type-${i}`}
								/>
							</div>
							{#if config.headers[i].required}
								<div class="flex w-full flex-col gap-1">
									<label for={`header-name-${i}`} class="text-sm font-light">Name</label>
									<input
										id={`header-name-${i}`}
										class="text-input-filled bg-background w-full shadow-none"
										bind:value={config.headers[i].name}
										disabled={readonly}
									/>
								</div>
								<div class="flex w-full flex-col gap-1">
									<label for={`header-description-${i}`} class="text-sm font-light"
										>Description</label
									>
									<input
										id={`header-description-${i}`}
										class="text-input-filled bg-background w-full shadow-none"
										bind:value={config.headers[i].description}
										disabled={readonly}
									/>
								</div>
								<div class="flex w-full flex-col gap-1">
									<label
										for={`header-prefix]-${i}`}
										class="flex items-center gap-1 text-sm font-light"
									>
										Value Prefix
										<InfoTooltip
											text="A constant prepended value that will be added to the user-supplied value. Ex. 'Bearer ' in 'Bearer [USER_SUPPLIED_VALUE]'."
											popoverWidth="lg"
										/>
									</label>
									<input
										id={`header-prefix-${i}`}
										class="text-input-filled bg-background w-full shadow-none"
										bind:value={config.headers[i].prefix}
										disabled={readonly}
									/>
								</div>
								<Toggle
									classes={{ label: 'text-sm text-inherit' }}
									disabled={readonly}
									label="Sensitive"
									labelInline
									checked={!!header.sensitive}
									onChange={(checked) => {
										if (config.headers?.[i]) {
											config.headers[i].sensitive = checked;
										}
									}}
								/>
							{:else}
								<input
									id={`header-description-${i}`}
									class="text-input-filled bg-background w-full shadow-none"
									bind:value={config.headers[i].value}
									disabled={readonly}
								/>
							{/if}
						</div>

						{#if !readonly}
							<button
								class="icon-button"
								onclick={() => {
									config.headers?.splice(i, 1);
								}}
								use:tooltip={'Delete Header'}
							>
								<Trash2 class="size-4" />
							</button>
						{/if}
					</div>
				{/each}
			{/if}
			{#if !readonly}
				<div class="flex justify-end">
					<button
						class="button flex items-center gap-1 text-xs"
						onclick={() => {
							if (!config.headers) {
								config.headers = [];
							}
							config.headers?.push({
								key: '',
								description: '',
								name: '',
								value: '',
								required: false,
								sensitive: false,
								file: false
							});
						}}
					>
						<Plus class="size-4" />
						Header
					</button>
				</div>
			{/if}
		</div>
	</div>
	<!-- Static OAuth Configuration -->
	{#if config}
		{@const remoteConfig = config as RemoteCatalogConfigAdmin}
		<div
			class="dark:bg-surface1 dark:border-surface3 bg-background flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
		>
			<div class="flex justify-between">
				<button
					type="button"
					class="flex grow cursor-pointer flex-col gap-1 text-left"
					disabled={readonly}
					onclick={() => {
						if (readonly) return;
						remoteConfig.staticOAuthRequired = !remoteConfig.staticOAuthRequired;
					}}
				>
					<div class="flex items-center gap-1">
						<h4
							class={twMerge(
								'text-sm font-semibold',
								!remoteConfig.staticOAuthRequired && 'opacity-50'
							)}
						>
							Static OAuth
						</h4>
					</div>
					<p class="text-on-surface1 text-xs font-light">
						Enable this if the remote MCP server requires OAuth authentication with a static client
						ID and secret.
					</p>
				</button>
				<div class="flex self-start">
					<Toggle
						classes={{ label: 'text-sm text-inherit' }}
						disabled={readonly}
						label={remoteConfig.staticOAuthRequired
							? 'Disable Static OAuth'
							: 'Enable Static OAuth'}
						checked={!!remoteConfig.staticOAuthRequired}
						onChange={(checked) => {
							remoteConfig.staticOAuthRequired = checked;
						}}
					/>
				</div>
			</div>

			{#if remoteConfig.staticOAuthRequired}
				<div in:slide={{ axis: 'y' }} class="flex flex-col gap-4">
					{#if isNewEntry}
						<div class="notification-info p-3 text-sm font-light">
							<div class="flex items-start gap-3">
								<Info class="mt-0.5 size-5 flex-shrink-0" />
								<p>You can provide OAuth credentials after saving.</p>
							</div>
						</div>
					{:else if onConfigureOAuth}
						<button
							class="button flex w-fit items-center gap-2 text-sm"
							onclick={onConfigureOAuth}
							disabled={readonly}
							type="button"
						>
							<Settings class="size-4" />
							Configure OAuth Credentials
						</button>
					{/if}
				</div>
			{/if}
		</div>
	{/if}
{/if}

<button
	class="button-text pl-0"
	onclick={() => {
		showAdvanced = !showAdvanced;

		if (!showAdvanced) {
			const catalogConfig = config as RemoteCatalogConfigAdmin;
			catalogConfig.hostname = undefined;
			catalogConfig.urlTemplate = undefined;
			catalogConfig.fixedURL = catalogConfig.fixedURL ?? '';
		}
	}}
>
	{showAdvanced ? 'Reset Default Configuration' : 'Advanced Configuration'}
</button>
