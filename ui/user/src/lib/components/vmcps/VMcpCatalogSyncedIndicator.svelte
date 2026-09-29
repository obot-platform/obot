<script lang="ts">
	import type { VMCP } from '$lib/services';
	import { isCatalogSyncedVMcp } from '$lib/services/vmcps/utils';
	import { isWebURL } from '$lib/url';
	import InfoTooltip from '../InfoTooltip.svelte';
	import type { Placement } from '@floating-ui/dom';
	import { FolderGit2 } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		vmcp: VMCP;
		placement?: Placement;
		class?: string;
		iconClass?: string;
	}

	let { vmcp, placement = 'bottom-start', class: klass, iconClass }: Props = $props();
</script>

{#if isCatalogSyncedVMcp(vmcp)}
	<InfoTooltip
		class={twMerge('size-4 shrink-0', klass)}
		classes={{ icon: twMerge('text-primary size-4', iconClass) }}
		icon={FolderGit2}
		ariaLabel="Synced from catalog"
		{placement}
		interactive
	>
		<div class="flex flex-col gap-1 text-left text-xs font-normal">
			<p class="font-semibold">Synced from catalog</p>
			<p>
				This vMCP is managed by a catalog source and is read-only in Obot. Make changes in the
				catalog source; they are applied on the next sync.
			</p>
			{#if vmcp.sourceURL}
				{#if isWebURL(vmcp.sourceURL)}
					<a
						href={vmcp.sourceURL}
						target="_blank"
						rel="external noopener noreferrer"
						class="text-link break-all"
					>
						{vmcp.sourceURL}
					</a>
				{:else}
					<p class="break-all">Source: {vmcp.sourceURL}</p>
				{/if}
			{/if}
		</div>
	</InfoTooltip>
{/if}
