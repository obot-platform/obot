<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import type { ImagePullSecret } from '$lib/services';
	import { timePreference } from '$lib/stores/index.js';
	import { formatTime } from '$lib/time.js';
	import { openUrl } from '$lib/utils.js';
	import { displayName, statusClass, statusLabel, statusMessage } from './types';
	import { Info, KeyRound, Plus, RefreshCw, ShieldCheck, Trash2 } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		imagePullSecrets: ImagePullSecret[];
		mutationsDisabled?: boolean;
		refreshing?: boolean;
		onCreate: () => void;
		onStatus: (secret: ImagePullSecret) => void;
		onTest: (secret: ImagePullSecret) => void;
		onRefresh: (secret: ImagePullSecret) => void;
		onDelete: (secret: ImagePullSecret) => void;
	}

	let {
		imagePullSecrets,
		mutationsDisabled = false,
		refreshing = false,
		onCreate,
		onStatus,
		onTest,
		onRefresh,
		onDelete
	}: Props = $props();

	let tableData = $derived(
		imagePullSecrets.map((item) => ({
			...item,
			displayName: displayName(item),
			detail:
				item.manifest.type === 'ecr'
					? (item.manifest.ecr?.region ?? '-')
					: (item.manifest.basic?.server ?? '-'),
			statusLabel: statusLabel(item),
			statusMessage: statusMessage(item),
			lastSuccess: item.status?.lastSuccessTime ?? ''
		}))
	);
	let basicSecrets = $derived(tableData.filter((item) => item.manifest.type === 'basic'));
	let ecrSecrets = $derived(tableData.filter((item) => item.manifest.type === 'ecr'));

	function formatDate(value?: string) {
		return value ? formatTime(value, timePreference.timeFormat) : '-';
	}
</script>

{#if imagePullSecrets.length === 0}
	<div class="mt-12 flex w-md max-w-full flex-col items-center gap-4 self-center text-center">
		<KeyRound class="text-on-surface1 size-24 opacity-25" />
		<h4 class="text-on-surface1 text-lg font-semibold">No image pull secrets</h4>
		<p class="text-on-surface1 text-sm font-light">
			Create a managed image pull secret to let Obot pull private MCP server images.
		</p>
		{#if !mutationsDisabled}
			<button class="button-primary flex items-center gap-1 text-sm" onclick={onCreate}>
				<Plus class="size-4" />
				Create New Secret
			</button>
		{/if}
	</div>
{:else}
	<div class="flex flex-col gap-8">
		{#if basicSecrets.length > 0}
			<section class="flex flex-col gap-3">
				<h2 class="text-on-surface1 text-lg font-semibold">Basic Secrets</h2>
				<Table
					data={basicSecrets}
					fields={['displayName', 'detail', 'id']}
					headers={[
						{ title: 'Name', property: 'displayName' },
						{ title: 'Registry', property: 'detail' },
						{ title: 'Secret', property: 'id' }
					]}
					sortable={['displayName', 'detail', 'id']}
					filterable={['displayName', 'detail']}
					onClickRow={(row, isCtrlClick) =>
						openUrl(`/admin/image-pull-secrets?id=${row.id}`, isCtrlClick)}
				>
					{#snippet actions(secret)}
						<div class="flex items-center gap-1">
							<button
								class="icon-button"
								disabled={mutationsDisabled}
								use:tooltip={'Test'}
								onclick={(e) => {
									e.stopPropagation();
									onTest(secret);
								}}
							>
								<ShieldCheck class="size-4" />
							</button>
							<button
								class="icon-button hover:text-red-500"
								disabled={mutationsDisabled}
								use:tooltip={'Delete'}
								onclick={(e) => {
									e.stopPropagation();
									onDelete(secret);
								}}
							>
								<Trash2 class="size-4" />
							</button>
						</div>
					{/snippet}
					{#snippet onRenderColumn(property, secret)}
						{#if property === 'displayName'}
							{displayName(secret)}
						{:else}
							{String(secret[property as keyof typeof secret] ?? '-')}
						{/if}
					{/snippet}
				</Table>
			</section>
		{/if}

		{#if ecrSecrets.length > 0}
			<section class="flex flex-col gap-3">
				<h2 class="text-on-surface1 text-lg font-semibold">ECR Secrets</h2>
				{@render ecrTable()}
			</section>
		{/if}
	</div>
{/if}

{#snippet ecrTable()}
	<Table
		data={ecrSecrets}
		fields={['displayName', 'detail', 'id', 'statusLabel', 'lastSuccess', 'statusMessage']}
		headers={[
			{ title: 'Name', property: 'displayName' },
			{ title: 'Region', property: 'detail' },
			{ title: 'Secret', property: 'id' },
			{ title: 'Status', property: 'statusLabel' },
			{ title: 'Last Success', property: 'lastSuccess' },
			{ title: 'Message', property: 'statusMessage' }
		]}
		sortable={['displayName', 'detail', 'id', 'statusLabel', 'lastSuccess']}
		filterable={['statusLabel']}
		onClickRow={(row, isCtrlClick) =>
			openUrl(`/admin/image-pull-secrets?id=${row.id}`, isCtrlClick)}
	>
		{#snippet actions(secret)}
			<div class="flex items-center gap-1">
				<button
					class="icon-button"
					use:tooltip={'Status'}
					onclick={(e) => {
						e.stopPropagation();
						onStatus(secret);
					}}
				>
					<Info class="size-4" />
				</button>
				<button
					class="icon-button"
					disabled={mutationsDisabled}
					use:tooltip={'Test'}
					onclick={(e) => {
						e.stopPropagation();
						onTest(secret);
					}}
				>
					<ShieldCheck class="size-4" />
				</button>
				<button
					class="icon-button"
					disabled={mutationsDisabled}
					use:tooltip={'Refresh now'}
					onclick={(e) => {
						e.stopPropagation();
						onRefresh(secret);
					}}
				>
					<RefreshCw class={twMerge('size-4', refreshing && 'animate-spin')} />
				</button>
				<button
					class="icon-button hover:text-red-500"
					disabled={mutationsDisabled}
					use:tooltip={'Delete'}
					onclick={(e) => {
						e.stopPropagation();
						onDelete(secret);
					}}
				>
					<Trash2 class="size-4" />
				</button>
			</div>
		{/snippet}
		{#snippet onRenderColumn(property, secret)}
			{#if property === 'displayName'}
				{displayName(secret)}
			{:else if property === 'statusLabel'}
				<span class={twMerge('rounded-full px-2 py-1 text-xs font-medium', statusClass(secret))}>
					{secret.statusLabel}
				</span>
			{:else if property === 'lastSuccess'}
				{formatDate(secret.status?.lastSuccessTime)}
			{:else if property === 'statusMessage'}
				{#if secret.statusMessage}
					<button
						class="line-clamp-2 max-w-sm text-left text-red-600 underline-offset-2 hover:underline dark:text-red-300"
						onclick={(e) => {
							e.stopPropagation();
							onStatus(secret);
						}}
					>
						{secret.statusMessage}
					</button>
				{:else}
					<span>-</span>
				{/if}
			{:else}
				{String(secret[property as keyof typeof secret] ?? '-')}
			{/if}
		{/snippet}
	</Table>
{/snippet}
