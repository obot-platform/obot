<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import type { MCPCatalogEntry, MCPCatalogServer, MCPServerInstance } from '$lib/services';
	import { responsive } from '$lib/stores';
	import { Link2Icon } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		connection: MCPCatalogServer | MCPServerInstance | MCPCatalogEntry;
		requiresConfiguration?: boolean;
		onClick?: () => void;
	}
	let { connection, requiresConfiguration, onClick }: Props = $props();
	let copyButton = $state<ReturnType<typeof CopyButton>>();
	let disabled = $derived(requiresConfiguration || !connection.connectURL);
</script>

{#if responsive.isMobile}
	<button
		class={twMerge(
			'btn btn-sm btn-primary border-none w-26',
			requiresConfiguration ? 'btn-soft bg-primary/10 hover:bg-primary' : ''
		)}
		onclick={(e) => {
			e.stopPropagation();
			onClick?.();
		}}
	>
		{requiresConfiguration ? 'Configure' : 'Connect'}
	</button>
{:else}
	<div
		role="presentation"
		onclick={(e) => e.stopPropagation()}
		class="w-full flex items-center gap-2"
	>
		<div class="rounded-field bg-base-200 border-none input w-full pr-0">
			<div class="label px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 mr-1">
				<Link2Icon class={twMerge('size-4', requiresConfiguration && 'opacity-50')} />
			</div>
			<input
				onmousedown={() => copyButton?.copy()}
				type="text"
				value={connection.connectURL ?? ''}
				class={twMerge('w-full text-xs', requiresConfiguration && 'opacity-50')}
				readonly
				{disabled}
			/>
			<div class="mr-2">
				<CopyButton
					bind:this={copyButton}
					classes={{
						button:
							'shrink-0 text-xs flex gap-1 :not([disabled]):hover:text-base-content :not([disabled]):text-base-content/65 disabled:cursor-not-allowed'
					}}
					text={connection.connectURL ?? ''}
					showTextLeft
					{disabled}
				/>
			</div>
		</div>
		<button
			class={twMerge(
				'btn btn-sm btn-primary border-none w-26',
				requiresConfiguration ? 'btn-soft bg-primary/10 hover:bg-primary' : ''
			)}
			onclick={onClick}
		>
			{requiresConfiguration ? 'Configure' : 'Connect'}
		</button>
	</div>
{/if}
