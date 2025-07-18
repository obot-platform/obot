<script lang="ts">
	import type { MCPServerInfo } from '$lib/services/chat/mcp';
	import { Server } from 'lucide-svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import type { Snippet } from 'svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import SensitiveInput from '../SensitiveInput.svelte';
	import { ChatService, type Project } from '$lib/services';

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
		onClose?: () => void;
		actions?: Snippet;
		catalogId?: string;
		catalogEntryId?: string;
		project?: Project;
	}
	let {
		form = $bindable(),
		onClose,
		name,
		icon,
		actions,
		catalogEntryId,
		project
	}: Props = $props();
	let configDialog = $state<ReturnType<typeof ResponsiveDialog>>();

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
	export function close() {
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
	{#if form}
		<div class="my-4 flex flex-col gap-4">
			{#if form.envs && form.envs.length > 0}
				{#each form.envs as env, i (env.key)}
					<div class="flex flex-col gap-1">
						<span class="flex items-center gap-2">
							<label for={env.key}>
								{env.name}
								{#if !env.required}
									<span class="text-gray-400 dark:text-gray-600">(optional)</span>
								{/if}
							</label>
							<InfoTooltip text={env.description} />
						</span>
						{#if env.sensitive}
							<SensitiveInput name={env.name} bind:value={form.envs[i].value} />
						{:else}
							<input
								type="text"
								id={env.key}
								bind:value={form.envs[i].value}
								class="text-input-filled"
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
				<input type="text" id="url-manifest-url" bind:value={form.url} class="text-input-filled" />
				{#if form.hostname}
					<span class="font-light text-gray-400 dark:text-gray-600">
						The URL must contain the hostname: <b class="font-semibold">
							{form.hostname}
						</b>
					</span>
				{/if}
			{/if}
		</div>
	{/if}

	{#if actions}
		{@render actions()}
	{/if}
</ResponsiveDialog>
