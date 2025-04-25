<script lang="ts">
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';
	import { Plus, Trash2 } from 'lucide-svelte';
	import type { Project } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea';

	interface Props {
		project: Project;
	}

	let { project }: Props = $props();
</script>

<CollapsePane header="Website Knowledge" compact>
	<p class="text-gray mb-4 text-xs">Add websites to your agent's knowledge base.</p>
	<div class="flex flex-col gap-2">
		{#if project.websiteKnowledge?.sites}
			<table class="w-full text-left">
				<thead class="text-sm">
					<tr>
						<th class="font-light">Website Address</th>
						<th class="font-light">Description</th>
					</tr>
				</thead>
				<tbody>
					{#each project.websiteKnowledge.sites as _, i (i)}
						<tr class="group">
							<td>
								<input
									bind:value={project.websiteKnowledge.sites[i].site}
									placeholder="example.com"
									class="ghost-input border-surface2 w-3/4"
								/>
							</td>
							<td>
								<textarea
									class="ghost-input border-surface2 w-5/6 resize-none"
									bind:value={project.websiteKnowledge.sites[i].description}
									rows="1"
									placeholder="Description"
									use:autoHeight
								></textarea>
							</td>
							<td class="flex justify-end">
								<button
									class="icon-button"
									onclick={() => {
										project.websiteKnowledge?.sites?.splice(i, 1);
									}}
								>
									<Trash2 class="size-5" />
								</button>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
		<div class="self-end">
			<button
				class="button-small"
				onclick={() => {
					if (!project.websiteKnowledge) {
						project.websiteKnowledge = {
							sites: [{}]
						};
					} else if (!project.websiteKnowledge.sites) {
						project.websiteKnowledge.sites = [{}];
					} else {
						project.websiteKnowledge.sites.push({});
					}
				}}
			>
				<Plus class="size-4" />
				Website
			</button>
		</div>
	</div>
</CollapsePane>
