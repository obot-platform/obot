<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import { hasSecretBinding, type MCPServerInfo } from '$lib/services/user/mcp';
	import { version } from '$lib/stores';
	import Confirm from '../Confirm.svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import Toggle from '../Toggle.svelte';
	import SecretBindingPicker from './SecretBindingPicker.svelte';
	import { CircleAlert, Server } from '@lucide/svelte';
	import { tick, type Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	export type LaunchFormData = {
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
		hostname?: string;
		name?: string;
	};

	export type ComponentLaunchFormData = {
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
		hostname?: string;
		name?: string;
		icon?: string;
		disabled?: boolean; // source of truth; checkbox shows Enable and binds to !disabled
		// When true, this component represents a multi-user server. Composite
		// configuration can still collect the component instance's user-specific
		// headers, but does not expose catalog-entry env/URL settings.
		isMultiUser?: boolean;
	};

	export type CompositeLaunchFormData = {
		componentConfigs: Record<string, ComponentLaunchFormData>;
		name?: string;
	};

	interface Props {
		form?: LaunchFormData | CompositeLaunchFormData;
		name?: string;
		icon?: string;
		onSave?: () => void;
		onCancel?: () => void;
		onClose?: () => void;
		actions?: Snippet;
		catalogId?: string;
		cancelText?: string;
		submitText?: string;
		loading?: boolean;
		loadingContent?: Snippet;
		error?: string;
		serverId?: string;
		isNew?: boolean;
		showAlias?: boolean;
		disableSave?: boolean;
		disableOutsideClick?: boolean;
		animate?: 'slide' | 'fade' | null;
		displayDescriptionInline?: boolean;
		configurationTitle?: string;
		secretBindingTargets?: MCPAllowedSecretBindingTarget[];
		disableEnvSecretBindings?: boolean;
	}
	let {
		form = $bindable(),
		onCancel,
		onClose,
		onSave,
		name,
		icon,
		cancelText = 'Cancel',
		submitText = 'Save',
		loading,
		loadingContent,
		error,
		isNew,
		showAlias,
		disableSave,
		disableOutsideClick,
		displayDescriptionInline,
		configurationTitle,
		secretBindingTargets,
		disableEnvSecretBindings,
		animate = 'slide'
	}: Props = $props();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let highlightedFields = $state<Set<string>>(new Set());
	let showConfirmClose = $state(false);
	let initialFormJson = $state<string>('');
	let resizing = $state(false);
	let compositeInfoDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let isOpen = $state(false);
	let localError = $state<string | undefined>();

	type ConfigField =
		| NonNullable<MCPServerInfo['env']>[number]
		| NonNullable<MCPServerInfo['headers']>[number];

	function isPinnedSecretBinding(field?: ConfigField) {
		return Boolean(
			(field as { secretBindingReadonly?: boolean } | undefined)?.secretBindingReadonly
		);
	}

	function usesSecretBindingSource(field?: ConfigField & { secretBindingSource?: string }) {
		return Boolean(field?.secretBinding) || field?.secretBindingSource === 'secret';
	}

	function fieldLabel(field: Partial<MCPSubField>) {
		return field.name || field.key || '';
	}

	const remoteHeaders = $derived.by(() => {
		if (form && 'headers' in form) {
			return getNonStaticServerFields(form?.headers);
		}

		return [];
	});

	const visibleEnvs = $derived.by(() => {
		if (form && 'envs' in form) {
			return getNonStaticServerFields(form?.envs);
		}

		return [];
	});

	export async function open() {
		await tick();
		if (isCompositeForm(form) && isNew) {
			compositeInfoDialog?.open();
		} else {
			openConfig();
		}
	}

	function openConfig() {
		configDialog?.open();
		localError = undefined;
		if (!isNew) {
			// store initial form data as jsonified string for comparison when not new
			initialFormJson = JSON.stringify(form);
		}

		isOpen = true;
	}

	function clearHighlights() {
		highlightedFields = new Set();
	}

	function startTextareaResize() {
		resizing = true;
		document.addEventListener('pointerup', stopTextareaResize, { once: true });
		document.addEventListener('mouseup', stopTextareaResize, { once: true });
	}

	function stopTextareaResize() {
		resizing = false;
	}

	function hasUrl(url?: string) {
		return url?.trim().length ?? 0 > 0;
	}

	function isCompositeForm(f: unknown): f is CompositeLaunchFormData {
		return (
			typeof f === 'object' && f !== null && 'componentConfigs' in (f as Record<string, unknown>)
		);
	}

	function hasAtLeastOneEnabled(formAny?: LaunchFormData | CompositeLaunchFormData) {
		if (!formAny) return false;
		if (isCompositeForm(formAny)) {
			return Object.values(formAny.componentConfigs || {}).some((c) => !c.disabled);
		}
		return true;
	}

	function keyFor(compId: string, k: string) {
		return `${compId}:${k}`;
	}

	function componentHasConfig(comp?: ComponentLaunchFormData) {
		if (!comp) return false;
		// Multi-user component entries should not expose any configuration
		// fields in this dialog; they are configured at the multi-user level.
		if (comp.isMultiUser) return false;
		const hasEnvs =
			Array.isArray(comp.envs) &&
			(secretBindingTargets !== undefined || comp.envs.some((e) => !hasSecretBinding(e)));
		const hasHeaders =
			Array.isArray(comp.headers) &&
			(secretBindingTargets !== undefined || comp.headers.some((h) => !hasSecretBinding(h)));
		const needsURL = Boolean(comp.hostname);
		return hasEnvs || hasHeaders || needsURL;
	}

	function isEditableField(field: Partial<MCPSubField> & { isStatic?: boolean }) {
		return !hasSecretBinding(field) && !field.isStatic;
	}

	function missingRequiredFields(formAny: LaunchFormData | CompositeLaunchFormData) {
		if (!formAny) return false;
		if (isCompositeForm(formAny)) {
			for (const comp of Object.values(formAny.componentConfigs || {})) {
				if (comp.disabled) continue;
				const envs = comp.envs ?? [];
				const headers = comp.headers ?? [];
				if (comp.hostname && !hasUrl(comp.url)) {
					return true;
				}
				if ([...envs, ...headers].some((f) => isEditableField(f) && f.required && !f.value)) {
					return true;
				}
			}
			return false;
		}

		const form = formAny as LaunchFormData;
		if (form.hostname && !hasUrl(form.url)) {
			return true;
		}
		const envs = form.envs ?? [];
		const headers = form.headers ?? [];
		return [...envs, ...headers].some(
			(field) => isEditableField(field) && field.required && !field.value
		);
	}

	function highlightMissingRequiredFields(formAny: LaunchFormData | CompositeLaunchFormData) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const fieldsToHighlight = new Set<string>();
		if (isCompositeForm(formAny)) {
			for (const [compId, comp] of Object.entries(formAny.componentConfigs || {})) {
				if (comp.disabled) continue;
				for (const f of comp.envs ?? []) {
					if (isEditableField(f) && f.required && !f.value)
						fieldsToHighlight.add(keyFor(compId, f.key));
				}
				for (const f of comp.headers ?? []) {
					if (isEditableField(f) && f.required && !f.value)
						fieldsToHighlight.add(keyFor(compId, f.key));
				}
				if (comp.hostname && !comp.url) fieldsToHighlight.add(keyFor(compId, 'url'));
			}
			highlightedFields = fieldsToHighlight;
			return;
		}
		const form = formAny as LaunchFormData;
		[...(form.envs ?? []), ...(form.headers ?? [])].forEach((field) => {
			if (isEditableField(field) && field.required && !field.value) {
				fieldsToHighlight.add(field.key);
			}
		});
		if (form.hostname && !form.url) {
			fieldsToHighlight.add('url-manifest-url');
		}
		highlightedFields = fieldsToHighlight;
	}

	function handleSave() {
		if (!form) return;

		localError = undefined;
		if (!hasAtLeastOneEnabled(form)) {
			localError = 'Please enable at least one component server.';
			return;
		}

		if (missingRequiredFields(form)) {
			highlightMissingRequiredFields(form);
			localError = 'Please fill out all required configuration fields.';
			return;
		}

		onSave?.();
	}

	export function close() {
		clearHighlights();
		initialFormJson = '';
		localError = undefined;
		compositeInfoDialog?.close();
		configDialog?.close();
		isOpen = false;
	}

	function hasFieldFilledOut(formAny?: LaunchFormData | CompositeLaunchFormData) {
		if (!formAny) return false;
		if (isCompositeForm(formAny)) {
			for (const comp of Object.values(formAny.componentConfigs || {})) {
				const hasEnvOrHeaderFilled = [...(comp.envs ?? []), ...(comp.headers ?? [])].some(
					(f) => isEditableField(f) && f.value
				);
				const hasHostnameAndUrl = comp.hostname && hasUrl(comp.url);
				if (hasEnvOrHeaderFilled || hasHostnameAndUrl) return true;
			}
			return false;
		}
		const form = formAny as LaunchFormData;
		const hasEnvOrHeaderFilled = [...(form.envs ?? []), ...(form.headers ?? [])].some(
			(field) => isEditableField(field) && field.value
		);
		const hasHostnameAndUrl = form.hostname && hasUrl(form.url);
		return hasEnvOrHeaderFilled || hasHostnameAndUrl;
	}

	function hasFormChanged() {
		if (!initialFormJson) return false;
		return JSON.stringify(form) !== initialFormJson;
	}

	function getNonStaticServerFields(
		fields: MCPServerInfo['headers'] | MCPServerInfo['env'] | undefined
	) {
		if (!fields) return [];

		return (
			fields
				.map((field, i) => ({
					index: i,
					data: field as typeof field & { isStatic?: boolean }
				}))
				?.filter((item) => !item.data.isStatic) ?? []
		);
	}
</script>

<ResponsiveDialog
	bind:this={compositeInfoDialog}
	{animate}
	title="MCP Composite Server"
	class="max-w-md"
>
	<p class="font-light">This MCP server is a composite of the following MCP servers:</p>
	{#if form && 'componentConfigs' in form}
		<div class="my-4 flex flex-col items-center justify-center gap-2">
			{#each Object.entries(form.componentConfigs) as [compId, comp] (compId)}
				<div class="flex items-center gap-2">
					{#if comp.icon}
						<img src={comp.icon} alt={comp.name || compId} class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
					<div class="font-xs font-semibold">{comp.name}</div>
				</div>
			{/each}
		</div>
	{/if}
	<p class="font-light">
		The composite server may require configuring each of the MCP servers or disabling/enabling which
		servers are included to match your needs.
	</p>
	<button
		class="btn btn-secondary mt-4"
		onclick={() => {
			compositeInfoDialog?.close();
			openConfig();
		}}
	>
		Continue
	</button>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={configDialog}
	{animate}
	onClose={() => {
		if (loading) return;
		clearHighlights();
		localError = undefined;
		onClose?.();
		isOpen = false;
	}}
	onClickOutside={() => {
		if (resizing || disableOutsideClick || loading) return;
		if ((isNew && hasFieldFilledOut(form)) || (!isNew && hasFormChanged())) {
			showConfirmClose = true;
		} else {
			configDialog?.close();
			isOpen = false;
		}
	}}
	class={isCompositeForm(form) ? 'bg-base-200 dark:bg-base-100' : ''}
	disableClickOutside={loading}
	hideClose={loading}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			<div class="bg-base-200 rounded-sm p-1 dark:bg-base-300">
				{#if icon}
					<img src={icon} alt={name} class="size-8" />
				{:else}
					<Server class="size-8" />
				{/if}
			</div>
			{name}
		</div>
	{/snippet}

	{#if isOpen}
		{#if loading && loadingContent}
			{@render loadingContent()}
		{:else}
			{@render content()}
		{/if}
	{/if}
</ResponsiveDialog>

{#snippet content()}
	{#if error || localError}
		<div class="notification-error flex items-center gap-2">
			<CircleAlert class="size-6 shrink-0 text-error" />
			<p class="flex flex-col text-sm font-light">
				<span class="font-semibold">Error:</span>
				<span>
					{error || localError}
				</span>
			</p>
		</div>
	{/if}
	{#if form}
		<form
			id="mcp-catalog-configure-form"
			onsubmit={(e) => {
				e.preventDefault();
				handleSave();
			}}
		>
			<div class="my-4 flex flex-col gap-4">
				{#if showAlias}
					<div class="flex flex-col gap-1">
						<span class="flex items-center gap-2">
							<label for="name"> Server Alias </label>
							<span class="text-muted-content">(optional)</span>
							<InfoTooltip
								text="Uses server name as default. Duplicate instances default to a number increment added at the end of name."
							/>
						</span>
						<input type="text" id="name" bind:value={form.name} class="text-input-filled" />
					</div>
				{/if}

				{#if 'componentConfigs' in form}
					{#each Object.entries(form.componentConfigs) as [compId, comp] (compId)}
						<div
							class="dark:bg-base-300 dark:border-base-400 bg-base-100 rounded-lg border border-transparent shadow-sm"
						>
							<div class="flex items-center gap-2 p-2">
								{#if comp.icon}
									<img src={comp.icon} alt={comp.name || compId} class="size-8" />
								{/if}
								<div class="grow font-medium">{comp.name || compId}</div>
								<Toggle
									checked={!form.componentConfigs[compId].disabled}
									onChange={(checked) => (form.componentConfigs[compId].disabled = !checked)}
									label="Enable"
									labelInline
									classes={{ label: 'text-sm gap-2' }}
								/>
							</div>
							{#if componentHasConfig(comp)}
								{@const headers = getNonStaticServerFields(comp.headers)}
								{@const envs = getNonStaticServerFields(comp.envs)}

								<div class="border-t border-base-300 p-3">
									{#each envs as env (env.data.key)}
										{#if secretBindingTargets !== undefined || !hasSecretBinding(env.data)}
											{@const highlightRequired =
												highlightedFields.has(`${compId}:${env.data.key}`) && !env.data.value}
											<div class="flex flex-col gap-1">
												<span class="flex items-center gap-2">
													<label
														for={`${compId}-${env.data.key}`}
														class={highlightRequired ? 'text-error' : ''}
													>
														{fieldLabel(env.data)}
														{#if !env.data.required}
															<span class="text-muted-content">(optional)</span>
														{/if}
													</label>
													{#if !displayDescriptionInline}
														<InfoTooltip text={env.data.description} />
													{/if}
												</span>
												{#if isPinnedSecretBinding(env.data)}
													<div
														class="bg-base-200 dark:bg-base-300 border-base-300 dark:border-base-400 flex flex-col gap-1 rounded-lg border p-3 text-sm shadow-inner"
													>
														<span class="text-muted-content text-xs font-light"
															>Kubernetes Secret</span
														>
														<span class="font-mono"
															>{env.data.secretBinding?.name} / {env.data.secretBinding?.key}</span
														>
													</div>
												{:else if secretBindingTargets && !version.current.hideK8sDetails}
													<SecretBindingPicker
														bind:field={comp.envs![env.index]}
														targets={secretBindingTargets}
														readonly={form.componentConfigs[compId].disabled}
													/>
												{/if}
												{#if usesSecretBindingSource(env.data)}
													<!-- Secret-bound value is selected above. -->
												{:else if env.data.sensitive}
													<SensitiveInput
														error={highlightRequired}
														name={env.data.name}
														bind:value={comp.envs![env.index].value}
														disabled={form.componentConfigs[compId].disabled}
														textarea={env.data.file}
														growable
													/>
												{:else if env.data.file}
													<textarea
														id={`${compId}-${env.data.key}`}
														bind:value={comp.envs![env.index].value}
														disabled={form.componentConfigs[compId].disabled}
														rows="8"
														class={twMerge(
															'text-input-filled h-32 min-h-32 resize-y overflow-auto whitespace-pre-wrap',
															highlightRequired &&
																'border-error bg-error/20 ring-error focus:ring-1'
														)}
														onpointerdown={startTextareaResize}
														onmousedown={startTextareaResize}
													></textarea>
												{:else}
													<input
														type="text"
														id={`${compId}-${env.data.key}`}
														bind:value={comp.envs![env.index].value}
														disabled={form.componentConfigs[compId].disabled}
														class={twMerge(
															'text-input-filled',
															highlightRequired &&
																'border-error bg-error/20 ring-error focus:ring-1'
														)}
													/>
												{/if}
												{#if displayDescriptionInline}
													<p class="text-muted-content text-xs font-light break-all">
														{env.data.description}
													</p>
												{/if}
											</div>
										{/if}
									{/each}

									{#each headers as header (header.data.key)}
										{#if secretBindingTargets !== undefined || !hasSecretBinding(header.data)}
											{@const highlightRequired =
												highlightedFields.has(`${compId}:${header.data.key}`) && !header.data.value}

											<div class="flex flex-col gap-1">
												<span class="flex items-center gap-2">
													<label
														for={`${compId}-${header.data.key}`}
														class={highlightRequired ? 'text-error' : ''}
													>
														{fieldLabel(header.data)}
														{#if !header.data.required}
															<span class="text-muted-content">(optional)</span>
														{/if}
													</label>
													{#if !displayDescriptionInline}
														<InfoTooltip text={header.data.description} />
													{/if}
												</span>
												{#if isPinnedSecretBinding(header.data)}
													<div
														class="bg-base-200 dark:bg-base-300 border-base-300 dark:border-base-400 flex flex-col gap-1 rounded-lg border p-3 text-sm shadow-inner"
													>
														<span class="text-muted-content text-xs font-light"
															>Kubernetes Secret</span
														>
														<span class="font-mono"
															>{header.data.secretBinding?.name} / {header.data.secretBinding
																?.key}</span
														>
													</div>
												{:else if secretBindingTargets && !version.current.hideK8sDetails}
													<SecretBindingPicker
														bind:field={comp.headers![header.index]}
														targets={secretBindingTargets}
														readonly={form.componentConfigs[compId].disabled}
													/>
												{/if}
												{#if usesSecretBindingSource(header.data)}
													<!-- Secret-bound value is selected above. -->
												{:else if header.data.sensitive}
													<SensitiveInput
														name={header.data.name}
														bind:value={comp.headers![header.index].value}
														disabled={form.componentConfigs[compId].disabled}
														error={highlightRequired}
													/>
												{:else}
													<input
														type="text"
														id={`${compId}-${header.data.key}`}
														bind:value={comp.headers![header.index].value}
														disabled={form.componentConfigs[compId].disabled}
														class={twMerge(
															'text-input-filled',
															highlightRequired &&
																'border-error bg-error/20 ring-error focus:ring-1'
														)}
													/>
												{/if}
												{#if displayDescriptionInline}
													<p class="text-muted-content text-xs font-light break-all">
														{header.data.description}
													</p>
												{/if}
											</div>
										{/if}
									{/each}

									{#if comp.hostname}
										{@const highlightRequired = highlightedFields.has(`${compId}:url`) && !comp.url}
										<label for={`${compId}-url`}> URL </label>
										<input
											type="text"
											id={`${compId}-url`}
											bind:value={comp.url}
											disabled={form.componentConfigs[compId].disabled}
											class={twMerge(
												'text-input-filled',
												highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
											)}
										/>
										<span class="text-muted-content font-light">
											The URL must contain the hostname: <b class="font-semibold">{comp.hostname}</b
											>
										</span>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				{:else}
					{#if configurationTitle}
						<h4 class="text-sm font-semibold">{configurationTitle}</h4>
					{/if}

					{#each visibleEnvs as env (env.data.key)}
						{#if secretBindingTargets !== undefined || !hasSecretBinding(env.data)}
							{@const highlightRequired = highlightedFields.has(env.data.key) && !env.data.value}
							<div class="flex flex-col gap-1">
								<span class="flex items-center gap-2">
									<label for={env.data.key} class={highlightRequired ? 'text-error' : ''}>
										{fieldLabel(env.data)}
										{#if !env.data.required}
											<span class="text-muted-content">(optional)</span>
										{/if}
									</label>
									{#if !displayDescriptionInline}
										<InfoTooltip text={env.data.description} />
									{/if}
								</span>
								{#if isPinnedSecretBinding(env.data)}
									<div
										class="bg-base-200 dark:bg-base-300 border-base-300 dark:border-base-400 flex flex-col gap-1 rounded-lg border p-3 text-sm shadow-inner"
									>
										<span class="text-muted-content text-xs font-light">Kubernetes Secret</span>
										<span class="font-mono"
											>{env.data.secretBinding?.name} / {env.data.secretBinding?.key}</span
										>
									</div>
								{:else if secretBindingTargets && !disableEnvSecretBindings && !version.current.hideK8sDetails}
									<SecretBindingPicker
										bind:field={form.envs![env.index]}
										targets={secretBindingTargets}
									/>
								{/if}
								{#if usesSecretBindingSource(env.data)}
									<!-- Secret-bound value is selected above. -->
								{:else if env.data.sensitive}
									<SensitiveInput
										error={highlightRequired}
										name={env.data.name}
										bind:value={form.envs![env.index].value}
										textarea={env.data.file}
										growable
									/>
								{:else if env.data.file}
									<textarea
										id={env.data.key}
										bind:value={form.envs![env.index].value}
										rows="8"
										class={twMerge(
											'text-input-filled h-32 min-h-32 resize-y overflow-auto whitespace-pre-wrap',
											highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
										)}
										onpointerdown={startTextareaResize}
										onmousedown={startTextareaResize}
									></textarea>
								{:else}
									<input
										type="text"
										id={env.data.key}
										bind:value={form.envs![env.index].value}
										class={twMerge(
											'text-input-filled',
											highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
										)}
									/>
								{/if}
								{#if displayDescriptionInline}
									<p class="text-muted-content text-xs font-light break-all">
										{env.data.description}
									</p>
								{/if}
							</div>
						{/if}
					{/each}

					{#each remoteHeaders as header (header.data.key)}
						{#if secretBindingTargets !== undefined || !hasSecretBinding(header.data)}
							{@const highlightRequired =
								highlightedFields.has(header.data.key) && !header.data.value}
							<div class="flex flex-col gap-1">
								<span class="flex items-center gap-2">
									<label for={header.data.key} class={highlightRequired ? 'text-error' : ''}>
										{fieldLabel(header.data)}
										{#if !header.data.required}
											<span class="text-muted-content">(optional)</span>
										{/if}
									</label>
									<InfoTooltip text={header.data.description} />
								</span>
								{#if isPinnedSecretBinding(header.data)}
									<div
										class="bg-base-200 dark:bg-base-300 border-base-300 dark:border-base-400 flex flex-col gap-1 rounded-lg border p-3 text-sm shadow-inner"
									>
										<span class="text-muted-content text-xs font-light">Kubernetes Secret</span>
										<span class="font-mono"
											>{header.data.secretBinding?.name} / {header.data.secretBinding?.key}</span
										>
									</div>
								{:else if secretBindingTargets && !version.current.hideK8sDetails}
									<SecretBindingPicker
										bind:field={form.headers![header.index]}
										targets={secretBindingTargets}
									/>
								{/if}
								{#if usesSecretBindingSource(header.data)}
									<!-- Secret-bound value is selected above. -->
								{:else if header.data.sensitive}
									<SensitiveInput
										error={highlightRequired}
										name={header.data.name}
										bind:value={form.headers![header.index].value}
									/>
								{:else}
									<input
										type="text"
										id={header.data.key}
										bind:value={form!.headers![header.index].value}
										class={twMerge(
											'text-input-filled',
											highlightRequired && 'border-error bg-error/20 ring-error focus:ring-1'
										)}
									/>
								{/if}
							</div>
						{/if}
					{/each}

					{#if form.hostname}
						<label for="url-manifest-url"> URL </label>
						<input
							type="text"
							id="url-manifest-url"
							bind:value={form.url}
							class="text-input-filled"
						/>
						<span class="text-muted-content font-light">
							The URL must contain the hostname: <b class="font-semibold">
								{form.hostname}
							</b>
						</span>
					{/if}
				{/if}
			</div>
		</form>
		<div class="flex justify-end gap-2">
			{#if onCancel}
				<button class="btn btn-secondary" onclick={onCancel} disabled={loading}>
					{cancelText}
				</button>
			{/if}
			<button
				class="btn btn-primary"
				type="submit"
				form="mcp-catalog-configure-form"
				disabled={loading || disableSave}
			>
				{#if loading}
					<Loading class="size-4" />
				{:else}
					{submitText}
				{/if}
			</button>
		</div>
	{/if}
{/snippet}

<Confirm
	show={showConfirmClose}
	onsuccess={async () => {
		showConfirmClose = false;
		configDialog?.close();
		isOpen = false;
	}}
	oncancel={() => (showConfirmClose = false)}
	type="info"
	title="Confirm Cancel"
>
	{#snippet msgContent()}
		<h3 class="text-base-content text-lg font-semibold wrap-break-word">
			Are you sure you want to exit?
		</h3>
	{/snippet}
	{#snippet note()}
		<p class="w-sm">
			It looks like you have started filling out the server information. You will have to fill out
			the form again to launch this server.
		</p>
	{/snippet}
</Confirm>
