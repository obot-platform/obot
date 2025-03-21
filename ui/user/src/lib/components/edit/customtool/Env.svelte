<script lang="ts">
	import { Plus, Minus } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';

	interface Props {
		envs: { key: string; value: string }[];
	}

	let { envs = $bindable([]) }: Props = $props();
	const masked = '•••••••';
</script>

<div class="bg-surface1 flex flex-col gap-4 rounded-lg p-5">
	<div class="flex">
		<h4 class="flex-1 text-xl font-semibold">Additional Environment Variables</h4>
		<button onclick={() => envs.push({ key: '', value: '' })}>
			<Plus class="size-5" />
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
						<td>
							<input
								bind:value={env.key}
								placeholder="eg. SAMPLE_KEY"
								class="ast bg-surface1 outline-none"
							/>
						</td>
						<td class="group">
							<textarea
								use:autoHeight
								placeholder="Enter value"
								class="bg-surface1 resize-none outline-none"
								rows="1"
								onfocus={(e) => {
									if (e.target instanceof HTMLTextAreaElement) {
										if (e.target.value === masked) {
											e.target.value = env.value;
										}
									}
								}}
								onblur={(e) => {
									if (e.target instanceof HTMLTextAreaElement) {
										if (e.target.value !== masked) {
											env.value = e.target.value;
											e.target.value = masked;
										}
									}
								}}
								oninput={(e) => {
									if (e.target instanceof HTMLTextAreaElement) {
										if (e.target.value !== masked) {
											env.value = e.target.value;
										}
									}
								}}>{masked}</textarea
							>
						</td>
						<td>
							<button onclick={() => envs.splice(i, 1)}>
								<Minus class="size-5" />
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
