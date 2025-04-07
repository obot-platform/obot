<script lang="ts">
	import { DEFAULT_PROJECT_NAME } from '$lib/constants';
	import { getProjectImage } from '$lib/image';
	import type { Project, ProjectShare, ToolReference } from '$lib/services';
	import { darkMode, responsive } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
	import ToolPill from './ToolPill.svelte';

	interface Props {
		project: Project | ProjectShare;
		tools: Map<string, ToolReference>;
		onclick?: () => void;
		class?: string;
	}

	const { project, tools, onclick, class: klass }: Props = $props();
</script>

{#snippet content()}
	<div
		class={twMerge(
			'bg-surface1 z-10 flex h-full w-full grow items-center rounded-xl p-4 shadow-md',
			klass
		)}
	>
		<img
			alt="obot logo"
			src={getProjectImage(project, darkMode.isDark)}
			class="flex size-24 flex-shrink-0 rounded-full shadow-md shadow-gray-500 dark:shadow-black"
		/>
		<div class="flex flex-col gap-2 pl-4">
			<h4 class="line-clamp-2 text-left text-base leading-5.5 font-semibold md:text-lg">
				{project.name || DEFAULT_PROJECT_NAME}
			</h4>

			<p
				class="line-clamp-3 flex text-left text-xs leading-4.5 font-light text-gray-500 md:text-sm"
			>
				{project.description || ''}
			</p>

			{#if 'tools' in project && project.tools}
				{@const maxToolsToShow = responsive.isMobile ? 2 : 3}
				<div class="flex flex-wrap items-center justify-start gap-2">
					{#each project.tools.slice(0, maxToolsToShow) as tool}
						{@const toolData = tools.get(tool)}
						{#if toolData}
							<ToolPill tool={toolData} />
						{/if}
					{/each}
					{#if project.tools.length > maxToolsToShow}
						<ToolPill
							tools={project.tools
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
		class="card featured-card relative bg-transparent hover:shadow-none"
	>
		{@render content()}
	</a>
{/if}

<style lang="postcss">
	.featured-card {
		&:after {
			content: '';
			z-index: 0;
			position: absolute;
			height: 100%;
			width: 100%;
			bottom: -4px;
			left: 0;
			transition: transform 0.2s ease-in-out;
			background-image: linear-gradient(
				to bottom right,
				var(--color-blue-400),
				var(--color-blue-600)
			);
			border-radius: var(--radius-xl);
		}

		&:hover {
			&:after {
				transform: rotate(4deg) scale(0.95);
			}
		}

		@media (min-width: 640px) {
			&:after {
				height: calc(100%);
				width: calc(100% - 24px);
				bottom: -8px;
				left: 8px;
			}

			&:hover {
				&:after {
					transform: rotate(-3deg) scale(1);
				}
			}
		}
	}
</style>
