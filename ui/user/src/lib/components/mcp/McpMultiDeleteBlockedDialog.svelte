<script lang="ts">
	import { resolve } from '$app/paths';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { MCPCompositeDeletionDependency } from '$lib/services';
	import { TriangleAlert, Server, LoaderCircle } from '@lucide/svelte';

	interface Props {
		show: boolean;
		dependencies?: MCPCompositeDeletionDependency[];
		onClose: () => void;
		/** Provide to offer a force delete that bypasses the composite dependency check. */
		onForceDelete?: () => void | Promise<void>;
		forcing?: boolean;
		entity?: 'server' | 'entry';
		/** How many objects are blocked, when a bulk delete was blocked on more than one. */
		blockedCount?: number;
	}

	let {
		show,
		dependencies = [],
		onClose,
		onForceDelete,
		forcing = false,
		entity = 'server',
		blockedCount = 1
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	const entityLabel = $derived(entity === 'entry' ? 'catalog entry' : 'server');
	const subject = $derived(
		blockedCount > 1 ? `${blockedCount} ${entityLabel}s are` : `This ${entityLabel} is`
	);
	const pronoun = $derived(blockedCount > 1 ? 'them' : 'it');

	const groupedLinks = $derived.by(() => {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const grouped = new Map<string, { name: string; icon?: string; hasConfigDep: boolean }>();

		for (const dep of dependencies) {
			let g = grouped.get(dep.catalogEntryID);
			if (!g) {
				g = { name: dep.name, icon: dep.icon, hasConfigDep: false };
				grouped.set(dep.catalogEntryID, g);
			}
			if (!dep.mcpServerID) {
				g.hasConfigDep = true;
			}
		}

		return (
			Array.from(grouped.entries())
				.map(([catalogEntryID, g]) => ({
					catalogEntryID,
					name: g.name,
					icon: g.icon,
					label: g.hasConfigDep ? 'Edit Configuration' : 'Update Instances',
					url: g.hasConfigDep
						? `/admin/mcp-catalog/c/${catalogEntryID}?view=configuration`
						: `/admin/mcp-catalog/c/${catalogEntryID}?view=server-instances`
				}))
				// Show Edit Configuration links first, then Upgrade Instances
				.sort(
					(a, b) =>
						Number(b.label === 'Edit Configuration') - Number(a.label === 'Edit Configuration')
				)
		);
	});

	$effect(() => {
		if (show) {
			dialog?.open();
		} else {
			dialog?.close();
		}
	});
</script>

<ResponsiveDialog bind:this={dialog} {onClose} onClickOutside={onClose} class="md:max-w-xl">
	<div class="default-scrollbar-thin flex flex-col gap-4 overflow-y-auto p-4">
		<div class="notification-alert mb-2 flex flex-col gap-2">
			<div class="flex gap-2">
				<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">Action Required</p>
			</div>
			<span class="text-left text-sm font-light wrap-break-word">
				{subject} still used as a component by the composites below. Remove {pronoun} from each of them
				and update all deployed instances.
			</span>
		</div>

		{#if groupedLinks.length > 0}
			<ul class="space-y-2 text-sm">
				{#each groupedLinks as dep (dep.catalogEntryID)}
					<li
						class="dark:bg-base-300 dark:border-base-400 bg-base-100 flex items-center justify-between gap-3 rounded-md border border-gray-200 p-3 shadow-sm"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if dep.icon}
								<img src={dep.icon} alt={dep.name} class="size-6 shrink-0" />
							{:else}
								<Server class="size-6 shrink-0" />
							{/if}
							<span class="truncate font-medium text-gray-900 dark:text-gray-100">
								{dep.name}
							</span>
						</div>
						<a
							href={resolve(dep.url as `/${string}`)}
							class="text-xs font-medium whitespace-nowrap text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
						>
							{dep.label}
						</a>
					</li>
				{/each}
			</ul>
		{/if}

		{#if onForceDelete}
			<div class="flex flex-col gap-3 border-t border-gray-200 pt-4 dark:border-gray-700">
				<span class="text-left text-xs font-light text-gray-500 dark:text-gray-400">
					Deleting anyway leaves these references in place. Each composite keeps serving its current
					component configuration and reports the component as missing until it is removed.
				</span>
				<div class="flex justify-end gap-2">
					<button class="btn btn-ghost" disabled={forcing} onclick={onClose}>Cancel</button>
					<button class="btn btn-error" disabled={forcing} onclick={() => onForceDelete?.()}>
						{#if forcing}
							<LoaderCircle class="size-4 animate-spin" />
						{/if}
						Delete Anyway
					</button>
				</div>
			</div>
		{/if}
	</div>
</ResponsiveDialog>
