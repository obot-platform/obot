<script lang="ts">
	import { Plus, Minus } from 'lucide-svelte';
	import { autoHeight } from '$lib/actions/textarea';
	import { fade } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';

	interface Props {
		envs: { key: string; value: string }[];
	}

	let { envs = $bindable([]) }: Props = $props();
	const masked = '•••••••';
</script>

<div
	class="dark:border-surface3 dark:bg-surface1 flex flex-col gap-4 rounded-lg bg-white p-5 shadow-sm dark:border"
>
	<div class="flex min-h-10 items-center">
		<h4 class="flex-1 text-lg font-semibold">Additional Environment Variables</h4>
		{#if envs.length === 0}
			<button
				transition:fade
				class="icon-button"
				onclick={() => envs.push({ key: '', value: '' })}
				use:tooltip={{ text: 'Add Environment Variable', disablePortal: true }}
			>
				<Plus class="size-5" />
			</button>
		{/if}
	</div>
	{#if envs.length !== 0}
		<table class="mb-3 w-full text-left">
			<thead>
				<tr>
					<th class="w-1/2 font-medium md:w-1/4">Name</th>
					<th class="w-1/2 font-medium md:w-full">Value</th>
				</tr>
			</thead>
			<tbody>
				{#each envs as env, i}
					<tr>
						<td class="pr-4 align-bottom">
							<input bind:value={env.key} placeholder="eg. SAMPLE_KEY" class="text-input w-full" />
						</td>
						<td class="group pr-4 align-bottom">
							<textarea
								use:autoHeight
								placeholder="Enter value"
								class="text-input-filled dark:bg-surface2 resize-none align-bottom"
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
							<button
								class="icon-button"
								onclick={() => envs.splice(i, 1)}
								use:tooltip={{ text: 'Remove Environment Variable', disablePortal: true }}
							>
								<Minus class="size-5" />
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
		<div class="flex justify-end">
			<button class="button-small" onclick={() => envs.push({ key: '', value: '' })}>
				<Plus class="size-4" /> Environment Variable
			</button>
		</div>
	{/if}
</div>
