<script lang="ts">
	import { AlertTriangle, Server } from 'lucide-svelte/icons';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type {
		MCPCompositeDeletionDependency,
		MCPCompositeDeletionDependencyError
	} from '$lib/services';

	interface Props {
		show: boolean;
		error?: MCPCompositeDeletionDependencyError;
		onClose: () => void;
	}

	let { show, error, onClose }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	const groupedLinks = $derived.by(() => {
		const deps: MCPCompositeDeletionDependency[] = error?.dependencies ?? [];

		const grouped = new Map<string, { name: string; icon?: string; hasConfigDep: boolean }>();

		for (const dep of deps) {
			const id = dep.catalogEntryID;
			let g = grouped.get(id);
			if (!g) {
				g = { name: dep.name, icon: dep.icon, hasConfigDep: false };
				grouped.set(id, g);
			}
			if (!dep.mcpServerID) {
				g.hasConfigDep = true;
			}
		}

		return (
			Array.from(grouped.entries())
				.map(([catalogEntryID, g]) => {
					const hasConfigDep = g.hasConfigDep;

					const url = hasConfigDep
						? `/admin/mcp-servers/c/${catalogEntryID}?view=configuration`
						: `/admin/mcp-servers/c/${catalogEntryID}?view=server-instances`;

					const label = hasConfigDep ? 'Edit Configuration' : 'Update Instances';

					return { catalogEntryID, name: g.name, icon: g.icon, url, label };
				})
				// Show Edit Configuration links first, then Upgrade Instances
				.sort((a, b) => {
					const aIsEdit = a.label === 'Edit Configuration';
					const bIsEdit = b.label === 'Edit Configuration';
					return Number(bIsEdit) - Number(aIsEdit);
				})
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
				<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">Action Required</p>
			</div>
			<span class="text-left text-sm font-light break-words">
				To delete this server, please remove it from the servers below and update all deployed
				instances.
			</span>
		</div>

		{#if groupedLinks.length > 0}
			<ul class="space-y-2 text-sm">
				{#each groupedLinks as dep (dep.catalogEntryID)}
					<li
						class="dark:bg-surface2 dark:border-surface3 flex items-center justify-between gap-3 rounded-md border border-gray-200 bg-white p-3 shadow-sm"
					>
						<div class="flex min-w-0 items-center gap-3">
							{#if dep.icon}
								<img src={dep.icon} alt={dep.name} class="size-6 flex-shrink-0" />
							{:else}
								<Server class="size-6 flex-shrink-0" />
							{/if}
							<span class="truncate font-medium text-gray-900 dark:text-gray-100">
								{dep.name}
							</span>
						</div>
						<a
							href={dep.url}
							class="text-xs font-medium whitespace-nowrap text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
						>
							{dep.label}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</ResponsiveDialog>
