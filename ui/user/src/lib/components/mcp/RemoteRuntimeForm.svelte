<script lang="ts">
	import type {
		RemoteCatalogConfigAdmin,
		RemoteRuntimeConfigAdmin
	} from '$lib/services/admin/types';
	import { Plus, Trash2, Info } from 'lucide-svelte';
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
	}
	let { config = $bindable(), readonly, showRequired, onFieldChange }: Props = $props();

	// For catalog entries, we show advanced config if hostname, urlTemplate, or headers exist
	// For servers, we always show the URL field (no advanced toggle needed)
	let showAdvanced = $state(
		Boolean(
			(config as RemoteCatalogConfigAdmin).hostname ||
				(config as RemoteCatalogConfigAdmin).urlTemplate ||
				(config.headers && config.headers.length > 0)
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
		class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		in:fade={{ duration: 200 }}
	>
		<label
			for="basic-url"
			class={twMerge('w-24 text-sm font-light', showRequired?.fixedURL && 'error')}>URL</label
		>
		<input
			id="basic-url"
			class={twMerge(
				'text-input-filled flex grow dark:bg-black',
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
			class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<div class="flex items-center gap-4 {readonly ? 'hidden' : ''}">
				<label for="remote-type" class="flex-shrink-0 text-sm font-light"
					>Restrict connections to:</label
				>
				<Select
					class="bg-surface1 dark:border-surface3 border border-transparent shadow-inner dark:bg-black"
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
							'text-input-filled flex grow dark:bg-black',
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
							'text-input-filled flex grow dark:bg-black',
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
								'text-input-filled flex grow dark:bg-black',
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
									> syntax in your URL template. Variables can be populated from the User Supplied
									Configuration section below.
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
	<!-- Static Headers Section -->
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<h4 class="text-sm font-semibold">Static Headers</h4>
			<p class="text-xs font-light text-gray-400 dark:text-gray-600">
				Header values that you provide now and will be sent with every request to the MCP server.
			</p>
			{#if config.headers}
				{@const staticHeaders = config.headers.filter((h) => !h.required)}
				{#each staticHeaders as header, idx (header)}
					{@const i = config.headers.indexOf(header)}
					<div
						class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
					>
						<div class="flex w-full flex-col gap-4">
							<div class="flex w-full flex-col gap-1">
								<label for={`static-header-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`static-header-key-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].key}
									placeholder="e.g. Authorization"
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`static-header-value-${i}`} class="text-sm font-light">Value</label>
								<input
									id={`static-header-value-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].value}
									placeholder="e.g. Bearer token123"
									disabled={readonly}
									type={config.headers[i].sensitive ? 'password' : 'text'}
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
						Static Header
					</button>
				</div>
			{/if}
		</div>
	</div>

	<!-- User Supplied Configuration Section -->
	<div class="flex w-full flex-col gap-8" in:slide>
		<div
			class="dark:bg-surface1 dark:border-surface3 flex flex-col gap-4 rounded-lg border border-transparent bg-white p-4 shadow-sm"
		>
			<h4 class="text-sm font-semibold">User Supplied Configuration</h4>
			<p class="text-xs font-light text-gray-400 dark:text-gray-600">
				{#if selectedType === 'urlTemplate'}
					Header values that will be provided by users during initial setup. These values can be used
					in URL template interpolation.
				{:else}
					Header values that will be provided by users during initial setup and sent with every
					request to the MCP server.
				{/if}
			</p>
			{#if config.headers}
				{@const userSuppliedHeaders = config.headers.filter((h) => h.required)}
				{#each userSuppliedHeaders as header, idx (header)}
					{@const i = config.headers.indexOf(header)}
					<div
						class="dark:border-surface3 flex w-full items-center gap-4 rounded-lg border border-transparent bg-gray-50 p-4 dark:bg-gray-900"
					>
						<div class="flex w-full flex-col gap-4">
							<div class="flex w-full flex-col gap-1">
								<label for={`user-header-key-${i}`} class="text-sm font-light">Key</label>
								<input
									id={`user-header-key-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].key}
									placeholder="e.g. CUSTOM_HEADER_KEY"
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`user-header-name-${i}`} class="text-sm font-light">Name</label>
								<input
									id={`user-header-name-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].name}
									placeholder="Display name shown to users"
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`user-header-description-${i}`} class="text-sm font-light"
									>Description</label
								>
								<input
									id={`user-header-description-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].description}
									placeholder="Help text shown to users"
									disabled={readonly}
								/>
							</div>
							<div class="flex w-full flex-col gap-1">
								<label for={`user-header-prefix-${i}`} class="flex items-center gap-1 text-sm font-light">
									Value Prefix
									<InfoTooltip
										text="A constant prepended value that will be added to the user-supplied value. Ex. 'Bearer ' in 'Bearer [USER_SUPPLIED_VALUE]'."
										popoverWidth="lg"
									/>
								</label>
								<input
									id={`user-header-prefix-${i}`}
									class="text-input-filled w-full"
									bind:value={config.headers[i].prefix}
									placeholder="e.g. Bearer "
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
								required: true,
								sensitive: false,
								file: false
							});
						}}
					>
						<Plus class="size-4" />
						User Configuration
					</button>
				</div>
			{/if}
		</div>
	</div>
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
