<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { AlertCircle, LoaderCircle, Server } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import type { Snippet } from 'svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import { ChatService, type Project } from '$lib/services';
	import { twMerge } from 'tailwind-merge';

	export type LaunchFormData = {
		envs?: MCPServerInfo['env'];
		headers?: MCPServerInfo['headers'];
		url?: string;
		hostname?: string;
	};

	interface Props {
		form?: LaunchFormData;
		name?: string;
		icon?: string;
		onSave: () => void;
		onCancel?: () => void;
		onClose?: () => void;
		actions?: Snippet;
		catalogId?: string;
		catalogEntryId?: string;
		project?: Project;
		cancelText?: string;
		submitText?: string;
		loading?: boolean;
		error?: string;
	}
	let {
		form = $bindable(),
		onClose,
		onCancel,
		onSave,
		name,
		icon,
		catalogEntryId,
		project,
		cancelText = 'Cancel',
		submitText = 'Save',
		loading,
		error
	}: Props = $props();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let highlightedFields = $state<Set<string>>(new Set());

	export function open() {
		configDialog?.open();

		if (catalogEntryId && project && form) {
			ChatService.revealProjectMCPEnvHeaders(project.assistantID, project.id, catalogEntryId).then(
				(envAndHeaders) => {
					if (form.envs) {
						for (const env of form.envs) {
							if (envAndHeaders[env.key]) {
								env.value = envAndHeaders[env.key];
							}
						}
					}
					if (form.headers) {
						for (const header of form.headers) {
							if (envAndHeaders[header.key]) {
								header.value = envAndHeaders[header.key];
							}
						}
					}
				}
			);
		}
	}

	function clearHighlights() {
		highlightedFields = new Set();
	}

	function missingRequiredFields(form: LaunchFormData) {
		if (!form) return false;

		if (form.hostname && !form.url) {
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

		onSave();
	}

	export function close() {
		clearHighlights();
		configDialog?.close();
	}
</script>

<ResponsiveDialog bind:this={configDialog} animate="slide" {onClose}>
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
	{#if error}
		<div class="notification-error flex items-center gap-2">
			<AlertCircle class="size-6 text-red-500" />
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
							{#if env.sensitive}
								<SensitiveInput
									error={highlightRequired}
									name={env.name}
									bind:value={form.envs[i].value}
								/>
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
				{#if form.headers && form.headers.length > 0}
					{#each form.headers as header, i (header.key)}
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
							{#if header.sensitive}
								<SensitiveInput name={header.name} bind:value={form.headers[i].value} />
							{:else}
								<input
									type="text"
									id={header.key}
									bind:value={form.headers[i].value}
									class="text-input-filled"
								/>
							{/if}
						</div>
					{/each}
				{/if}
				{#if form.url}
					<label for="url-manifest-url"> URL </label>
					<input
						type="text"
						id="url-manifest-url"
						bind:value={form.url}
						class="text-input-filled"
					/>
					{#if form.hostname}
						<span class="font-light text-gray-400 dark:text-gray-600">
							The URL must contain the hostname: <b class="font-semibold">
								{form.hostname}
							</b>
						</span>
					{/if}
				{/if}
			</div>
		</form>
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
