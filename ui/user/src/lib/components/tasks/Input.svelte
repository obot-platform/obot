<script lang="ts">
	import { type Task } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';

	interface Props {
		input?: string;
		displayRunID?: string;
		task?: Task;
	}

	let { input = $bindable(''), task }: Props = $props();

	let params: Record<string, string> = $state({});
	let payload: string = $state('');
	let emailInput = $state({
		type: 'email',
		from: '',
		to: '',
		subject: '',
		body: ''
	});

	$effect(() => {
		if (task?.onDemand?.params) {
			input = JSON.stringify(params);
		} else if (task?.email) {
			input = JSON.stringify(emailInput);
		} else if (task?.webhook) {
			input = JSON.stringify({
				type: 'webhook',
				payload
			});
		} else {
			input = '';
		}
	});
</script>

<div class="border-surface2 dark:border-surface3 relative w-full rounded-lg border-2 p-5 pt-2">
	<h4
		class="dark:bg-surface2 absolute top-0 left-3 w-fit -translate-y-3.5 bg-white px-2 text-base font-semibold"
	>
		{#if task?.onDemand?.params}
			Arguments
		{:else if task?.email}
			Sample Email Details
		{:else if task?.webhook}
			Sample Webhook Payload
		{/if}
	</h4>

	{#if task?.onDemand?.params}
		<div class="mt-4 flex flex-col items-baseline gap-4">
			{#each Object.keys(task.onDemand.params) as key}
				<div class="flex w-full flex-col gap-1">
					<label for="param-{key}" class="flex-1 text-sm font-medium capitalize">{key}</label>
					<input
						id="param-{key}"
						bind:value={params[key]}
						class="text-input-filled"
						placeholder={task.onDemand.params[key]}
					/>
				</div>
			{/each}
		</div>
	{:else if task?.email}
		<div class="mt-4 flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">From</label>
				<input id="from" bind:value={emailInput.from} class="text-input-filled" placeholder="" />
			</div>
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">To</label>
				<input id="from" bind:value={emailInput.to} class="text-input-filled" placeholder="" />
			</div>
			<div class="flex flex-col gap-1">
				<label for="from" class="w-[70px] text-sm font-light">Subject</label>
				<input id="from" bind:value={emailInput.subject} class="text-input-filled" placeholder="" />
			</div>
			<div class="flex">
				<textarea
					id="body"
					bind:value={emailInput.body}
					use:autoHeight
					rows="1"
					class="text-input-filled resize-none p-5"
					placeholder="Email content"
				></textarea>
			</div>
		</div>
	{:else if task?.webhook}
		<textarea
			bind:value={payload}
			use:autoHeight
			rows="1"
			class="text-input-filled mt-2 w-full resize-none p-5"
			placeholder="Enter payload..."
		></textarea>
	{/if}
</div>
