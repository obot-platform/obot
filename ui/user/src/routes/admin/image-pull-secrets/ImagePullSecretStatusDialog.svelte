<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { ImagePullSecret } from '$lib/services';
	import { timePreference } from '$lib/stores/index.js';
	import { formatTime } from '$lib/time.js';
	import { displayName, statusLabel, statusMessage } from './types';
	import { CircleAlert, Clock, History, LoaderCircle, Server } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		secret?: ImagePullSecret;
		details?: ImagePullSecret;
		loading?: boolean;
		error?: string;
		onClose?: () => void;
	}

	let { secret, details, loading = false, error = '', onClose }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	type IconComponent = typeof Server;

	export function open() {
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}

	function formatDate(value?: string) {
		return value ? formatTime(value, timePreference.timeFormat) : '-';
	}

	function statusTone(value?: string) {
		switch (value) {
			case 'Ready':
				return 'bg-green-500/10 text-green-700 dark:text-green-300';
			case 'Error':
				return 'bg-red-500/10 text-red-700 dark:text-red-300';
			case 'Disabled':
				return 'bg-gray-500/10 text-gray-600 dark:text-gray-300';
			default:
				return 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-300';
		}
	}

	function shouldBadge(label: string) {
		return label === 'Status';
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	title={`Status${secret ? `: ${displayName(secret)}` : ''}`}
	class="w-full md:max-w-4xl"
	{onClose}
>
	<div class="m-4 flex flex-col gap-5">
		{#if loading}
			<div class="notification-info flex items-center gap-3 text-sm">
				<LoaderCircle class="size-5 animate-spin" />
				<span>Loading status...</span>
			</div>
		{:else if error}
			<div class="notification-error flex items-center gap-3 text-sm">
				<CircleAlert class="size-5" />
				<span>{error}</span>
			</div>
		{:else if details}
			<section class="flex flex-col gap-3">
				<div class="grid gap-3 md:grid-cols-3">
					{@render statusValue('Status', statusLabel(details), Server)}
					{@render statusValue('Last Success', formatDate(details.status?.lastSuccessTime), History)}
					{@render statusValue('Token Expires', formatDate(details.status?.tokenExpiresAt), Clock)}
				</div>
			</section>

			{#if statusMessage(details)}
				<div
					class="rounded-lg border border-red-500 bg-red-500/10 p-4 text-red-700 dark:text-red-300"
				>
					<div class="mb-3 flex items-center justify-between gap-2">
						<div class="flex items-center gap-2 text-sm font-semibold">
							<CircleAlert class="size-4" />
							Last Error
						</div>
						<CopyButton text={statusMessage(details)} />
					</div>
					<pre class="whitespace-pre-wrap wrap-break-word text-sm">{statusMessage(details)}</pre>
				</div>
			{/if}

			{#if details.status?.registryEndpoints?.length}
				<section class="flex flex-col gap-3">
					{@render sectionHeader('Registry Endpoints')}
					<div
						class="dark:bg-background dark:border-surface3 bg-surface1 flex flex-col gap-2 rounded-lg border border-transparent p-3 shadow-sm"
					>
						{#each details.status.registryEndpoints as endpoint (endpoint)}
							<div
								class="dark:bg-surface2 bg-background text-on-surface1 rounded-md px-3 py-2 font-mono text-xs break-all"
							>
								{endpoint}
							</div>
						{/each}
					</div>
				</section>
			{/if}
		{/if}
	</div>
</ResponsiveDialog>

{#snippet sectionHeader(title: string)}
	<h4 class="text-on-surface1 text-sm font-semibold">{title}</h4>
{/snippet}

{#snippet statusValue(label: string, value?: string, Icon?: IconComponent)}
	<div
		class="dark:bg-background dark:border-surface3 bg-surface1 flex min-w-0 items-start gap-3 rounded-lg border border-transparent p-3 shadow-sm"
	>
		{#if Icon}
			<div
				class="dark:bg-surface2 bg-background text-on-surface1 flex size-8 shrink-0 items-center justify-center rounded-md"
			>
				<Icon class="size-4" />
			</div>
		{/if}
		<div class="min-w-0">
			<div class="text-on-surface1 text-xs font-medium">{label}</div>
			{#if shouldBadge(label)}
				<div
					class={twMerge(
						'mt-1 inline-flex max-w-full rounded-full px-2 py-0.5 text-xs font-medium',
						statusTone(value)
					)}
				>
					<span class="truncate">{value || '-'}</span>
				</div>
			{:else}
				<div class="text-on-background mt-1 break-words text-sm">{value || '-'}</div>
			{/if}
		</div>
	</div>
{/snippet}
