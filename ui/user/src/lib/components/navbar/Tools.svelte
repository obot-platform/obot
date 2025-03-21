<script lang="ts">
	import { Wrench } from 'lucide-svelte/icons';
	import { type AssistantTool, type Project } from '$lib/services';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { responsive } from '$lib/stores';

	interface Prop {
		project: Project;
		tools: AssistantTool[];
	}

	let { tools }: Prop = $props();
</script>

<Menu
	title="Tools"
	description="AI can use the following tools."
	showRefresh={false}
	classes={{
		button: 'button-icon-primary',
		dialog: responsive.isMobile
			? 'rounded-none max-h-[calc(100vh-64px)] left-0 bottom-0 w-full'
			: ''
	}}
	slide={responsive.isMobile ? 'up' : undefined}
	fixed={responsive.isMobile}
>
	{#snippet icon()}
		<Wrench class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		<ul class="space-y-4 py-6 text-sm">
			{#each tools as tool, i}
				{#if !tool.builtin && tool.enabled}
					<li>
						<div class="flex">
							{#if tool.icon}
								<img
									class="h-8 w-8 rounded-md bg-gray-100 p-1"
									src={tool.icon}
									alt="message icon"
								/>
							{:else}
								<Wrench class="h-8 w-8 rounded-md bg-gray-100 p-1 text-black" />
							{/if}
							<div class="flex flex-1 px-2">
								<div>
									<label for="checkbox-item-{i}" class="text-sm font-medium dark:text-gray-100"
										>{tool.name}</label
									>
									<p class="text-gray text-xs font-normal dark:text-gray-300">
										{tool.description}
									</p>
								</div>
							</div>
						</div>
					</li>
				{/if}
			{/each}
		</ul>
	{/snippet}
</Menu>
