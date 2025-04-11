<script lang="ts">
	import { type Project, type ProjectShare, type ToolReference } from '$lib/services';
	import { darkMode, responsive } from '$lib/stores';
	import { getProjectImage } from '$lib/image';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import { DEFAULT_PROJECT_NAME, IGNORED_BUILTIN_TOOLS } from '$lib/constants';
	import type { Snippet } from 'svelte';
	import { UserPen } from 'lucide-svelte';

	interface Props {
		project: Project | ProjectShare;
		tools: Map<string, ToolReference>;
		actionContent?: Snippet;
	}
	let { project, tools, actionContent }: Props = $props();
	const toolsToShow = $derived((project.tools ?? []).filter((t) => !IGNORED_BUILTIN_TOOLS.has(t)));
</script>

<a
	href={'publicID' in project ? `/s/${project.publicID}` : `/o/${project.id}`}
	data-sveltekit-preload-data={'publicID' in project ? 'off' : 'hover'}
	class="card group relative z-20 flex-col overflow-hidden shadow-md"
>
	<div class="flex h-fit w-full flex-col gap-2 p-4 md:h-auto md:grow">
		<div class="flex w-full">
			<img
				alt="obot logo"
				src={getProjectImage(project, darkMode.isDark)}
				class="size-18 rounded-full"
			/>
			<div class="flex grow flex-col justify-between gap-2 pl-3">
				<h4 class="text-md leading-4.5 font-semibold">
					{project.name || DEFAULT_PROJECT_NAME}
				</h4>
				<p class="line-clamp-3 grow text-xs font-light text-gray-500">
					{project.description}
				</p>
			</div>
			{#if !('publicID' in project) && actionContent}
				<div class="translate-x-2 -translate-y-2">
					{@render actionContent()}
				</div>
			{/if}
		</div>
		<div class="flex w-full justify-between">
			{#if 'editor' in project && project.editor}
				<span
					class="bg-surface2 mt-auto flex h-fit w-fit gap-1 rounded-full px-3 py-1 text-xs font-light text-gray-500"
				>
					<UserPen class="size-4" /> Owner
				</span>
			{/if}
			{#if toolsToShow.length > 0}
				{@const maxToolsToShow = responsive.isMobile ? 2 : 3}
				<div class="mt-auto flex w-full flex-wrap justify-end gap-2">
					{#each toolsToShow.slice(0, maxToolsToShow) as tool}
						{@const toolData = tools.get(tool)}
						{#if toolData}
							<ToolPill tool={toolData} />
						{/if}
					{/each}
					{#if toolsToShow.length > maxToolsToShow}
						<ToolPill
							tools={toolsToShow
								.slice(maxToolsToShow)
								.map((t) => tools.get(t))
								.filter((t): t is ToolReference => !!t)}
						/>
					{/if}
				</div>
			{/if}
		</div>
	</div>
</a>
