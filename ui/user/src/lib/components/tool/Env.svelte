<script lang="ts" module>
	export const masked = '•••••••';
</script>

<script lang="ts">
	import { Plus, Minus } from 'lucide-svelte';
	import { tick } from 'svelte';
	import { resize } from '$lib/actions/textarea';
	import { autoHeight } from '$lib/actions/textarea';

	interface Props {
		envs: { key: string; value: string; editing: string }[];
	}

	let { envs = $bindable([]) }: Props = $props();
</script>

<div class="flex flex-col gap-4 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
	<div class="flex">
		<h4 class="flex-1 text-xl font-semibold">Additional Environment Variables</h4>
		<button onclick={() => envs.push({ key: '', value: '', editing: '' })}>
			<Plus class="h-5 w-5" />
		</button>
	</div>
	{#if envs.length !== 0}
		<table class="w-full text-left">
			<thead>
				<tr>
					<th class="w-1/4">Name</th>
					<th class="w-full">Value</th>
				</tr>
			</thead>
			<tbody>
				{#each envs as env, i}
					<tr>
						<td
							><input
								bind:value={env.key}
								placeholder="eg. SAMPLE_KEY"
								class="ast bg-gray-50 outline-none dark:bg-gray-950"
							/></td
						>
						<td
							><textarea
								use:autoHeight
								placeholder="Enter value"
								class="resize-none bg-gray-50 outline-none dark:bg-gray-950"
								rows="1"
								bind:value={env.editing}
								onfocusin={(e) => {
									if (env.editing === masked) {
										env.editing = env.value;
										const t = e.target;
										if (t instanceof HTMLTextAreaElement) {
											tick().then(() => resize(t));
										}
									}
								}}
								onfocusout={(e) => {
									if (env.editing !== masked) {
										env.value = env.editing;
										env.editing = masked;
										const t = e.target;
										if (t instanceof HTMLTextAreaElement) {
											tick().then(() => resize(t));
										}
									}
								}}
							></textarea></td
						>
						<td>
							<button onclick={() => envs.splice(i, 1)}>
								<Minus class="h-5 w-5" />
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
