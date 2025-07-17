<script lang="ts">
	import { autoHeight } from '$lib/actions/textarea';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import type { OnDemand } from '$lib/services';
	import { Plus, Trash2 } from 'lucide-svelte';

	interface Props {
		onDemand?: OnDemand;
		readOnly?: boolean;
	}

	let { onDemand = $bindable(), readOnly }: Props = $props();
	let order = $state<string[]>([]);

	let hasParams = $derived(!!Object.keys(onDemand?.params ?? {}).length);

	$effect(() => {
		for (const key in onDemand?.params ?? {}) {
			if (!order.includes(key)) {
				order.push(key);
			}
		}
	});
</script>

<div class="flex flex-col gap-4">
	<h4 class="text-base font-medium">Arguments</h4>
	<p class="text-gray text-sm">
		If your task has arguments, you’ll be prompted to enter them when running the task manually from
		the UI and if the task is invoked from a chat thread or another task, the project will supply
		the arguments based on its current context and instructions.
	</p>
	<p class="text-gray text-sm">
		You can reference these arguments in your steps using
		<span class="font-mono text-black dark:text-white">$name</span> syntax, like “Crawl $website and
		find pages it links to.”
	</p>

	{#if hasParams}
		<table class="w-full text-left">
			<thead class="text-sm">
				<tr>
					<th class="font-medium">Name</th>
					<th class="font-medium">Description</th>
				</tr>
			</thead>
			<tbody>
				{#each order as key, i (i)}
					<tr class="group">
						<td class="w-1/2 pr-4 align-bottom">
							<input
								value={key}
								placeholder="Enter Name"
								class="text-input w-full"
								oninput={(e) => {
									if (e.target instanceof HTMLInputElement && onDemand?.params) {
										const oldKey = order[i];
										const newKey = e.target.value;
										order[i] = newKey;
										onDemand.params[newKey] = onDemand.params[oldKey] ?? '';
										delete onDemand.params[oldKey];
									}
								}}
								disabled={readOnly}
							/>
						</td>
						<td class="w-1/2 pr-4 align-bottom">
							<textarea
								use:autoHeight
								bind:value={onDemand!.params![order[i]]}
								class="text-input w-full resize-none py-2.5 align-bottom"
								disabled={readOnly}
								placeholder="Add a good description"
								rows="1"
							></textarea>
						</td>
						{#if !readOnly}
							<td class="flex justify-end">
								<button
									class="icon-button"
									onclick={() => {
										const key = order[i];

										if (onDemand?.params) {
											delete onDemand.params[key];
										}

										order = order.filter((k) => k !== key);
									}}
									use:tooltip={'Remove Argument'}
								>
									<Trash2 class="size-5" />
								</button>
							</td>
						{/if}
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}

	{#if !readOnly}
		<div class="self-end">
			<button
				class="button-small"
				onclick={() => {
					if (!onDemand?.params) {
						onDemand = {
							params: { '': '' }
						};
					}

					order.push('');
				}}
			>
				<Plus class="size-4" /> Argument
			</button>
		</div>
	{/if}
</div>
