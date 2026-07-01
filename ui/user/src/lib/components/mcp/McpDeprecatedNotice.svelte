<script lang="ts">
	import { isDeprecatedMCPServer } from '$lib/services/user/mcp';
	import { TriangleAlert } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	type DeprecatedMCPItem = {
		manifest?: {
			metadata?: {
				deprecated?: string;
			};
		};
	};

	interface Props {
		item?: DeprecatedMCPItem | null;
		deprecated?: boolean;
		variant?: 'badge' | 'notification';
		child?: boolean;
		class?: string;
	}

	let { item, deprecated, variant = 'badge', child = false, class: className }: Props = $props();

	let isDeprecated = $derived(deprecated ?? isDeprecatedMCPServer(item));
	let subject = $derived(child ? 'component server' : 'server');
	let replacement = $derived(child ? 'component' : 'server');
</script>

{#if isDeprecated}
	{#if variant === 'notification'}
		<div
			class={twMerge(
				'border-warning bg-warning/10 flex w-full items-start gap-2 rounded-md border p-3 text-left',
				className
			)}
		>
			<TriangleAlert class="text-warning mt-0.5 size-4 shrink-0" />
			<div class="text-sm">
				<p class="font-medium">This {subject} is deprecated.</p>
				<p class="text-muted-content">
					It may stop receiving updates or be removed in a future catalog release. Use a replacement
					{replacement} when possible.
				</p>
			</div>
		</div>
	{:else}
		<span
			class={twMerge('badge badge-xs border-warning text-warning gap-1 bg-warning/10', className)}
		>
			<TriangleAlert class="size-3" />
			Deprecated
		</span>
	{/if}
{/if}
