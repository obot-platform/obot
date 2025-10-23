<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { AlertCircle, LoaderCircle, Server } from 'lucide-svelte';
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

	interface Props {
		form?: LaunchFormData;
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

	let isOpen = $state(false);

	export function open() {
		configDialog?.open();
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

	function missingRequiredFields(form: LaunchFormData) {
		if (!form) return false;

		if (form.hostname && !hasUrl(form.url)) {
			return true;
		}

		const envs = form.envs ?? [];
		const headers = form.headers ?? [];
		return [...envs, ...headers].some((field) => field.required && !field.value);
	}

	function highlightMissingRequiredFields(form: LaunchFormData) {
		const fieldsToHighlight = new Set<string>();

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

		if (missingRequiredFields(form)) {
			highlightMissingRequiredFields(form);
			return;
		}

		// if prefix in field, make sure prefix is prepended to value before calling onSave
		form.envs = form.envs?.map((env) => {
			if (env.prefix && !env.value.startsWith(env.prefix)) {
				env.value = env.prefix + env.value;
			}
			return env;
		});
		form.headers = form.headers?.map((header) => {
			if (header.prefix && !header.value.startsWith(header.prefix)) {
				header.value = header.prefix + header.value;
			}
			return header;
		});
		onSave?.();
	}

	export function close() {
		clearHighlights();
		initialFormJson = '';
		configDialog?.close();
	}

	function hasFieldFilledOut(form?: LaunchFormData) {
		if (!form) return false;

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
</script>

<ResponsiveDialog
	bind:this={configDialog}
	{animate}
	onClose={() => {
		clearHighlights();
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
		{#if error}
			<div class="notification-error flex items-center gap-2">
				<AlertCircle class="size-6 flex-shrink-0 text-red-500" />
				<p class="flex flex-col text-sm font-light">
					<span class="font-semibold">Error:</span>
					<span>
						{error}
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
								<span class="text-gray-400 dark:text-gray-600">(optional)</span>
								<InfoTooltip
									text="Uses server name as default. Duplicate instances default to a number increment added at the end of name."
								/>
							</span>
							<input type="text" id="name" bind:value={form.name} class="text-input-filled" />
						</div>
					{/if}
					{#if form.envs && form.envs.length > 0}
						{#each form.envs as env, i (env.key)}
							{@const highlightRequired = highlightedFields.has(env.key) && !env.value}
							<div class="flex flex-col gap-1">
								<span class="flex items-center gap-2">
									<label for={env.key} class={highlightRequired ? 'text-red-500' : ''}>
										{env.name}
										{#if !env.required}
											<span class="text-gray-400 dark:text-gray-600">(optional)</span>
										{/if}
									</label>
									<InfoTooltip text={env.description} />
								</span>
								<div class="flex items-center">
									{#if env.prefix}
										<div class="text-input-filled w-fit rounded-r-none border-r-0">
											{env.prefix}
										</div>
									{/if}
									{#if env.sensitive}
										<SensitiveInput
											error={highlightRequired}
											name={env.name}
											bind:value={form.envs[i].value}
											textarea={env.file}
											growable
											class={env.prefix ? 'rounded-l-none' : ''}
										/>
									{:else if env.file}
										<textarea
											id={env.key}
											bind:value={form.envs[i].value}
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
											id={env.key}
											bind:value={form.envs[i].value}
											class={twMerge(
												'text-input-filled',
												env.prefix ? 'rounded-l-none' : '',
												highlightRequired &&
													'border-red-500 bg-red-500/20 ring-red-500 focus:ring-1'
											)}
										/>
									{/if}
								</div>
							</div>
						{/each}
					{/if}
					{#if form.headers && form.headers.length > 0}
						{#each form.headers as header, i (header.key)}
							{#if header.required}
								<div class="flex flex-col gap-1">
									<span class="flex items-center gap-2">
										<label for={header.key}>
											{header.name}
											{#if !header.required}
												<span class="text-gray-400 dark:text-gray-600">(optional)</span>
											{/if}
										</label>
										<InfoTooltip text={header.description} />
									</span>
									<div class="flex items-center">
										{#if header.prefix}
											<div class="text-input-filled w-fit rounded-r-none border-r-0">
												{header.prefix}
											</div>
										{/if}
										{#if header.sensitive}
											<SensitiveInput
												name={header.name}
												bind:value={form.headers[i].value}
												class={header.prefix ? 'rounded-l-none' : ''}
											/>
										{:else}
											<input
												type="text"
												id={header.key}
												bind:value={form.headers[i].value}
												class={twMerge('text-input-filled', header.prefix ? 'rounded-l-none' : '')}
											/>
										{/if}
									</div>
									{#if header.prefix && form.headers[i].value.startsWith(header.prefix)}
										<div class="mt-1 text-xs font-light text-gray-400 dark:text-gray-600">
											It looks like your value starts with the already supplied prefix. The value
											that will be saved will be "<span class="font-normal text-blue-500"
												>{header.prefix}
												{form.headers[i].value}</span
											>".
										</div>
									{/if}
								</div>
							{/if}
						{/each}
					{/if}
					{#if form.hostname}
						<label for="url-manifest-url"> URL </label>
						<input
							type="text"
							id="url-manifest-url"
							bind:value={form.url}
							class="text-input-filled"
						/>
						<span class="font-light text-gray-400 dark:text-gray-600">
							The URL must contain the hostname: <b class="font-semibold">
								{form.hostname}
							</b>
						</span>
					{/if}
				</div>
			</form>
		{/if}
	{/if}

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
</ResponsiveDialog>

<Confirm
	show={showConfirmClose}
	onsuccess={async () => {
		showConfirmClose = false;
		configDialog?.close();
		isOpen = false;
	}}
	oncancel={() => (showConfirmClose = false)}
>
	{#snippet title()}
		<h3 class="mb-5 text-lg font-semibold break-words text-black dark:text-gray-100">
			Are you sure you want to exit?
		</h3>
	{/snippet}
	{#snippet note()}
		<p class="mb-8 w-sm">
			It looks like you have started filling out the server information. You will have to fill out
			the form again to launch this server.
		</p>
	{/snippet}
</Confirm>
