<script lang="ts">
	import { DEFAULT_PROJECT_NAME, IGNORED_BUILTIN_TOOLS } from '$lib/constants';
	import { getProjectImage } from '$lib/image';
	import type { Project, ProjectShare, ToolReference } from '$lib/services';
	import { darkMode, responsive } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import ToolPill from '$lib/components/ToolPill.svelte';
	import type { Snippet } from 'svelte';
	import { sortShownToolsPriority } from '$lib/sort';

	interface Props {
		project: Project | ProjectShare;
		tools?: Map<string, ToolReference>;
		onclick?: () => void;
		class?: string;
		content?: Snippet;
	}

	const { project, tools, onclick, class: klass, content: overrideContent }: Props = $props();

	const toolsToShow = $derived(
		(project.tools ?? []).filter((t) => !IGNORED_BUILTIN_TOOLS.has(t)).sort(sortShownToolsPriority)
	);
</script>

{#snippet content()}
	<div class={twMerge('bg-surface1 z-10 flex h-full w-full grow  rounded-xl p-3 shadow-md', klass)}>
		{#if overrideContent}
			{@render overrideContent()}
		{:else}
			<div
				class="size-12 flex-shrink-0 overflow-hidden rounded-full bg-white p-2 shadow-sm dark:bg-gray-600 dark:shadow-black"
			>
				<img alt="obot logo" src={getProjectImage(project, darkMode.isDark)} />
			</div>
			<div class="flex grow flex-col gap-2 pl-2">
				<h4 class="line-clamp-2 text-left text-base leading-5 font-semibold">
					{project.name || DEFAULT_PROJECT_NAME}
				</h4>

				<p class="line-clamp-3 flex grow text-left text-sm leading-4.5 font-light text-gray-500">
					{project.description || ''}
				</p>

				{#if 'tools' in project && project.tools && tools}
					{@const maxToolsToShow = responsive.isMobile ? 2 : 3}
					<div class="flex flex-wrap items-center justify-end gap-2">
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
				{:else}
					<div class="min-h-2"></div>
					<!-- placeholder -->
				{/if}
			</div>
		{/if}
	</div>
{/snippet}

{#if onclick}
	<button {onclick} class="card featured-card relative bg-transparent hover:shadow-none">
		{@render content()}
	</button>
{:else}
	<a
		href={'publicID' in project ? `/s/${project.publicID}` : `/o/${project.id}`}
		data-sveltekit-preload-data={'publicID' in project ? 'off' : 'hover'}
		class="card relative bg-transparent hover:shadow-none"
	>
		{@render content()}
	</a>
{/if}
