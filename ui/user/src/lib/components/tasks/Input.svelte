<script lang="ts">
	import { ChatService, type Task } from '$lib/services';
	import { autoHeight } from '$lib/actions/textarea.js';
	import { currentAssistant } from '$lib/stores';

	interface Props {
		editMode?: boolean;
		input?: string;
		displayRunID?: string;
		task?: Task;
	}

	let { editMode = false, input = $bindable(''), task, displayRunID }: Props = $props();
	let show: boolean = $derived.by(() => {
		if (task?.schedule) {
			return false;
		}
		if (task?.webhook || task?.email) {
			return true;
		}
		return Object.keys(task?.onDemand?.params ?? {}).length > 0;
	});
	let params: Record<string, string> = $state({});
	let payload: string = $state('');
	let currentDisplayRunID: string = $state('');
	let emailInput = $state({
		type: 'email',
		from: '',
		to: '',
		subject: '',
		body: ''
	});
	let titlePrefix = $derived(displayRunID !== '' ? '' : 'Test Input ');
	let readonly = $derived(!!displayRunID);

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

	$effect(display);

	function display() {
		if (editMode || !displayRunID || !$currentAssistant.id || !task?.id) {
			currentDisplayRunID = '';
			return;
		}

		if (currentDisplayRunID === displayRunID) {
			return;
		}

		ChatService.getTaskRun($currentAssistant.id, task.id, displayRunID).then((taskRun) => {
			if (!taskRun?.input) {
				return;
			}

			try {
				const inputObj = JSON.parse(taskRun.input);
				if (inputObj.type === 'email') {
					emailInput = inputObj;
				} else if (inputObj.type === 'webhook') {
					payload = inputObj.payload;
				} else {
					params = inputObj;
				}
			} catch {
				// ignore
			}
		});

		currentDisplayRunID = displayRunID;
	}
</script>

{#if editMode || displayRunID}
	{#if show}
		<div class="mt-8 rounded-3xl bg-gray-50 p-5 dark:bg-gray-950">
			{#if task?.onDemand?.params}
				<h4 class="mb-3 text-xl font-semibold">{titlePrefix}Parameters</h4>
				{#each Object.keys(task.onDemand.params) as key}
					<div class="flex items-baseline">
						<label for="param-{key}" class="text-sm font-semibold capitalize">{key}</label>
						<input
							id="param-{key}"
							{readonly}
							bind:value={params[key]}
							class="rounded-md bg-gray-50 p-2 outline-none dark:bg-gray-950"
							placeholder={editMode ? 'Enter value' : 'No value'}
						/>
					</div>
				{/each}
			{:else if task?.email}
				<h4 class="text-xl font-semibold">{titlePrefix}Email</h4>
				<div class="mt-5 flex flex-col gap-5 rounded-3xl bg-white p-5 dark:bg-black">
					<div class="flex items-baseline">
						<label for="from" class="w-[70px] text-sm font-semibold">From</label>
						<input
							id="from"
							{readonly}
							bind:value={emailInput.from}
							class="rounded-md bg-gray-50 p-2 outline-none dark:bg-gray-950"
							placeholder=""
						/>
					</div>
					<div class="flex items-baseline">
						<label for="from" class="w-[70px] text-sm font-semibold">To</label>
						<input
							id="from"
							{readonly}
							bind:value={emailInput.to}
							class="rounded-md bg-gray-50 p-2 outline-none dark:bg-gray-950"
							placeholder=""
						/>
					</div>
					<div class="flex items-baseline">
						<label for="from" class="w-[70px] text-sm font-semibold">Subject</label>
						<input
							id="from"
							{readonly}
							bind:value={emailInput.subject}
							class="rounded-md bg-gray-50 p-2 outline-none dark:bg-gray-950"
							placeholder=""
						/>
					</div>
					<div class="flex">
						<textarea
							id="body"
							bind:value={emailInput.body}
							{readonly}
							use:autoHeight
							rows="1"
							class="mt-2 w-full resize-none rounded-3xl bg-gray-50 p-5 outline-none dark:bg-gray-950"
							placeholder="Email content"
						></textarea>
					</div>
				</div>
			{:else if task?.webhook}
				<h3 class="text-lg font-semibold">{titlePrefix}Webhook Payload</h3>
				<textarea
					bind:value={payload}
					use:autoHeight
					{readonly}
					rows="1"
					class="mt-2 w-full resize-none rounded-md bg-gray-50 p-2 outline-none dark:bg-gray-950"
					placeholder={editMode ? 'Enter payload...' : 'No payload'}
				></textarea>
			{/if}
		</div>
	{/if}
{/if}
