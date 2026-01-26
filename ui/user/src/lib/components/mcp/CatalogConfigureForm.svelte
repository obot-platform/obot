<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { AlertCircle, LoaderCircle, Server } from 'lucide-svelte';
	import Toggle from '../Toggle.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import type { Snippet } from 'svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import { twMerge } from 'tailwind-merge';
	import Confirm from '../Confirm.svelte';

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
		// When true, this component represents a multi-user server that is already
		// configured at the org/admin level. In composite configuration flows we
		// only allow toggling enable/disable, and hide all per-user config fields.
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
		disableOutsideClick?: boolean;
		animate?: 'slide' | 'fade' | null;
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
		disableOutsideClick,
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

	const remoteHeaders = $derived.by(() => {
		if (form && 'headers' in form) {
			return getNonStaticServerHeaders(form?.headers);
		}

		return [];
	});

	export function open() {
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
		// Multi-user component servers should not expose any configuration
		// fields in this dialog; they are configured at the multi-user level.
		if (comp.isMultiUser) return false;
		const hasEnvs = Array.isArray(comp.envs) && comp.envs.length > 0;
		const hasHeaders = Array.isArray(comp.headers) && comp.headers.length > 0;
		const needsURL = Boolean(comp.hostname);
		return hasEnvs || hasHeaders || needsURL;
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
				if ([...envs, ...headers].some((f) => f.required && !f.value)) {
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
		return [...envs, ...headers].some((field) => field.required && !field.value);
	}

	function highlightMissingRequiredFields(formAny: LaunchFormData | CompositeLaunchFormData) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const fieldsToHighlight = new Set<string>();
		if (isCompositeForm(formAny)) {
			for (const [compId, comp] of Object.entries(formAny.componentConfigs || {})) {
				if (comp.disabled) continue;
				for (const f of comp.envs ?? []) {
					if (f.required && !f.value) fieldsToHighlight.add(keyFor(compId, f.key));
				}
				for (const f of comp.headers ?? []) {
					if (f.required && !f.value) fieldsToHighlight.add(keyFor(compId, f.key));
				}
				if (comp.hostname && !comp.url) fieldsToHighlight.add(keyFor(compId, 'url'));
			}
			highlightedFields = fieldsToHighlight;
			return;
		}
		const form = formAny as LaunchFormData;
		[...(form.envs ?? []), ...(form.headers ?? [])].forEach((field) => {
			if (field.required && !field.value) {
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
			return;
		}

		onSave?.();
	}

	export function close() {
		clearHighlights();
		initialFormJson = '';
		localError = undefined;
		configDialog?.close();
	}

	function hasFieldFilledOut(formAny?: LaunchFormData | CompositeLaunchFormData) {
		if (!formAny) return false;
		if (isCompositeForm(formAny)) {
			for (const comp of Object.values(formAny.componentConfigs || {})) {
				const hasEnvOrHeaderFilled = [...(comp.envs ?? []), ...(comp.headers ?? [])].some(
					(f) => f.value
				);
				const hasHostnameAndUrl = comp.hostname && hasUrl(comp.url);
				if (hasEnvOrHeaderFilled || hasHostnameAndUrl) return true;
			}
			return false;
		}
		const form = formAny as LaunchFormData;
		const hasEnvOrHeaderFilled = [...(form.envs ?? []), ...(form.headers ?? [])].some(
			(field) => field.value
		);
		const hasHostnameAndUrl = form.hostname && hasUrl(form.url);
		return hasEnvOrHeaderFilled || hasHostnameAndUrl;
	}

	function hasFormChanged() {
		if (!initialFormJson) return false;
		return JSON.stringify(form) !== initialFormJson;
	}

	function getNonStaticServerHeaders(headers: MCPServerInfo['headers'] | undefined) {
		if (!headers) return [];

		return (
			headers
				.map((header, i) => ({
					index: i,
					data: header as typeof header & { isStatic?: boolean }
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
		class="button mt-4"
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
		clearHighlights();
		localError = undefined;
		onClose?.();
		isOpen = false;
	}}
	onClickOutside={() => {
		if (resizing || disableOutsideClick) return;
		if ((isNew && hasFieldFilledOut(form)) || (!isNew && hasFormChanged())) {
			showConfirmClose = true;
		} else {
			configDialog?.close();
			isOpen = false;
		}
	}}
	class={isCompositeForm(form) ? 'bg-surface1 dark:bg-background' : ''}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
			<div class="bg-surface1 rounded-sm p-1 dark:bg-gray-600">
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
			<AlertCircle class="size-6 flex-shrink-0 text-red-500" />
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
			onsubmit={(e) => {
				e.preventDefault();
			}}
		>
			<div class="my-4 flex flex-col gap-4">
				{#if showAlias}
					<div class="flex flex-col gap-1">
						<span class="flex items-center gap-2">
							<label for="name"> Server Alias </label>
							<span class="text-on-surface1">(optional)</span>
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
							class="dark:bg-surface2 dark:border-surface3 bg-background rounded-lg border border-transparent shadow-sm"
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
								{@const headers = getNonStaticServerHeaders(comp.headers)}

								<div class="border-t border-gray-200 p-3">
									{#if comp.envs && comp.envs.length > 0}
										{#each comp.envs as env, i (env.key)}
											{@const highlightRequired =
												highlightedFields.has(`${compId}:${env.key}`) && !env.value}
											<div class="flex flex-col gap-1">
												<span class="flex items-center gap-2">
													<label
														for={`${compId}-${env.key}`}
														class={highlightRequired ? 'text-red-500' : ''}
													>
														{env.name}
														{#if !env.required}
															<span class="text-on-surface1">(optional)</span>
														{/if}
													</label>
													<InfoTooltip text={env.description} />
												</span>
												{#if env.sensitive}
													<SensitiveInput
														error={highlightRequired}
														name={env.name}
														bind:value={comp.envs[i].value}
														disabled={form.componentConfigs[compId].disabled}
														textarea={env.file}
														growable
													/>
												{:else if env.file}
													<textarea
														id={`${compId}-${env.key}`}
														bind:value={comp.envs[i].value}
														disabled={form.componentConfigs[compId].disabled}
														class={twMerge(
															'text-input-filled h-32 resize-y whitespace-pre-wrap',
															highlightRequired &&
																'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
														)}
														onmousedown={() => (resizing = true)}
														onmouseup={() => (resizing = false)}
													></textarea>
												{:else}
													<input
														type="text"
														id={`${compId}-${env.key}`}
														bind:value={comp.envs[i].value}
														disabled={form.componentConfigs[compId].disabled}
														class={twMerge(
															'text-input-filled',
															highlightRequired &&
																'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
														)}
													/>
												{/if}
											</div>
										{/each}
									{/if}

									{#each headers as header (header.data.key)}
										{@const highlightRequired =
											highlightedFields.has(`${compId}:${header.data.key}`) && !header.data.value}

										<div class="flex flex-col gap-1">
											<span class="flex items-center gap-2">
												<label
													for={`${compId}-${header.data.key}`}
													class={highlightRequired ? 'text-red-500' : ''}
												>
													{header.data.name}
													{#if !header.data.required}
														<span class="text-on-surface1">(optional)</span>
													{/if}
												</label>
												<InfoTooltip text={header.data.description} />
											</span>
											{#if header.data.sensitive}
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
															'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
													)}
												/>
											{/if}
										</div>
									{/each}

									{#if comp.hostname}
										<label for={`${compId}-url`}> URL </label>
										<input
											type="text"
											id={`${compId}-url`}
											bind:value={comp.url}
											disabled={form.componentConfigs[compId].disabled}
											class="text-input-filled"
										/>
										<span class="text-on-surface1 font-light">
											The URL must contain the hostname: <b class="font-semibold">{comp.hostname}</b
											>
										</span>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				{:else}
					{#if form.envs && form.envs.length > 0}
						{#each form.envs as env, i (env.key)}
							{@const highlightRequired = highlightedFields.has(env.key) && !env.value}
							<div class="flex flex-col gap-1">
								<span class="flex items-center gap-2">
									<label for={env.key} class={highlightRequired ? 'text-red-500' : ''}>
										{env.name}
										{#if !env.required}
											<span class="text-on-surface1">(optional)</span>
										{/if}
									</label>
									<InfoTooltip text={env.description} />
								</span>
								{#if env.sensitive}
									<SensitiveInput
										error={highlightRequired}
										name={env.name}
										bind:value={form.envs[i].value}
										textarea={env.file}
										growable
									/>
								{:else if env.file}
									<textarea
										id={env.key}
										bind:value={form.envs[i].value}
										class={twMerge(
											'text-input-filled h-32 resize-y whitespace-pre-wrap',
											highlightRequired && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
										)}
										onmousedown={() => (resizing = true)}
										onmouseup={() => (resizing = false)}
									></textarea>
								{:else}
									<input
										type="text"
										id={env.key}
										bind:value={form.envs[i].value}
										class={twMerge(
											'text-input-filled',
											highlightRequired && 'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
										)}
									/>
								{/if}
							</div>
						{/each}
					{/if}

					{#each remoteHeaders as header (header.data.key)}
						<div class="flex flex-col gap-1">
							<span class="flex items-center gap-2">
								<label for={header.data.key}>
									{header.data.name}
									{#if !header.data.required}
										<span class="text-on-surface1">(optional)</span>
									{/if}
								</label>
								<InfoTooltip text={header.data.description} />
							</span>
							{#if header.data.sensitive}
								<SensitiveInput
									name={header.data.name}
									bind:value={form.headers![header.index].value}
								/>
							{:else}
								<input
									type="text"
									id={header.data.key}
									bind:value={form!.headers![header.index].value}
									class="text-input-filled"
								/>
							{/if}
						</div>
					{/each}

					{#if form.hostname}
						<label for="url-manifest-url"> URL </label>
						<input
							type="text"
							id="url-manifest-url"
							bind:value={form.url}
							class="text-input-filled"
						/>
						<span class="text-on-surface1 font-light">
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
				<button class="button" onclick={onCancel}>
					{cancelText}
				</button>
			{/if}
			<button class="button-primary" onclick={handleSave} disabled={loading}>
				{#if loading}
					<LoaderCircle class="size-4 animate-spin" />
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
		<h3 class="text-on-background text-lg font-semibold break-words">
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
