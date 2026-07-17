<script lang="ts">
	import type { HostedAgentQuestion, HostedAgentQuestionType } from '$lib/services/admin/types';
	import IconButton from '../primitives/IconButton.svelte';
	import { Plus, Trash2 } from '@lucide/svelte';

	interface Props {
		questions: HostedAgentQuestion[];
		readonly?: boolean;
	}

	let { questions = $bindable([]), readonly }: Props = $props();

	const TYPES: { value: HostedAgentQuestionType; label: string; hint: string }[] = [
		{ value: 'string', label: 'Text', hint: '' },
		{ value: 'number', label: 'Number', hint: '' },
		{ value: 'boolean', label: 'Yes / No', hint: 'true or false' },
		{ value: 'select', label: 'Choice', hint: 'One of the options below' },
		{ value: 'schedule', label: 'Schedule', hint: 'Cron expression, e.g. 0 3 * * *' }
	];

	function addQuestion() {
		questions = [
			...questions,
			{ key: '', name: '', description: '', type: 'string', required: false, default: '' }
		];
	}

	function removeQuestion(index: number) {
		questions = questions.filter((_, i) => i !== index);
	}

	// Options are only meaningful for a choice, and the API rejects them on any
	// other type, so drop them when the type changes away from select.
	function onTypeChange(index: number, type: HostedAgentQuestionType) {
		questions[index].type = type;
		if (type !== 'select') {
			questions[index].options = undefined;
		} else if (!questions[index].options) {
			questions[index].options = [''];
		}
		questions[index].default = '';
	}

	function addOption(index: number) {
		questions[index].options = [...(questions[index].options ?? []), ''];
	}

	function removeOption(qIndex: number, oIndex: number) {
		questions[qIndex].options = (questions[qIndex].options ?? []).filter((_, i) => i !== oIndex);
	}

	function typeHint(type: HostedAgentQuestionType | undefined) {
		return TYPES.find((t) => t.value === (type ?? 'string'))?.hint ?? '';
	}
</script>

<div class="flex flex-col gap-2">
	<div class="mb-2 flex items-center justify-between">
		<div class="flex flex-col">
			<h2 class="text-lg font-semibold">Questions</h2>
			<span class="text-muted-content text-xs">
				Asked when a user creates an instance of this agent.
			</span>
		</div>
		{#if !readonly}
			<button class="btn btn-primary flex items-center gap-1 text-sm" onclick={addQuestion}>
				<Plus class="size-4" /> Add Question
			</button>
		{/if}
	</div>

	{#if questions.length === 0}
		<p class="text-muted-content py-4 text-center text-sm">No questions added.</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each questions as question, i (i)}
				<div
					class="dark:bg-base-400 dark:border-base-400 bg-base-100 flex flex-col gap-3 rounded-lg border border-transparent p-4"
				>
					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="q-key-{i}" class="text-sm font-light">Key</label>
							<input
								id="q-key-{i}"
								bind:value={question.key}
								class="text-input-filled"
								placeholder="schedule"
								disabled={readonly}
							/>
						</div>
						<div class="flex flex-1 flex-col gap-2">
							<label for="q-name-{i}" class="text-sm font-light">Label</label>
							<input
								id="q-name-{i}"
								bind:value={question.name}
								class="text-input-filled"
								placeholder="Schedule"
								disabled={readonly}
							/>
						</div>
						<div class="flex flex-col gap-2">
							<label for="q-type-{i}" class="text-sm font-light">Type</label>
							<select
								id="q-type-{i}"
								class="text-input-filled"
								value={question.type ?? 'string'}
								onchange={(e) => onTypeChange(i, e.currentTarget.value as HostedAgentQuestionType)}
								disabled={readonly}
							>
								{#each TYPES as t (t.value)}
									<option value={t.value}>{t.label}</option>
								{/each}
							</select>
						</div>
						{#if !readonly}
							<IconButton
								variant="danger"
								onclick={() => removeQuestion(i)}
								tooltip={{ text: 'Remove Question' }}
							>
								<Trash2 class="size-4" />
							</IconButton>
						{/if}
					</div>

					<div class="flex items-end gap-3">
						<div class="flex flex-1 flex-col gap-2">
							<label for="q-desc-{i}" class="text-sm font-light">Description</label>
							<input
								id="q-desc-{i}"
								bind:value={question.description}
								class="text-input-filled"
								disabled={readonly}
							/>
						</div>
						<div class="flex flex-1 flex-col gap-2">
							<label for="q-default-{i}" class="text-sm font-light">Default</label>
							{#if question.type === 'select'}
								<select
									id="q-default-{i}"
									class="text-input-filled"
									bind:value={question.default}
									disabled={readonly}
								>
									<option value="">(none)</option>
									{#each (question.options ?? []).filter((o) => o) as option (option)}
										<option value={option}>{option}</option>
									{/each}
								</select>
							{:else if question.type === 'boolean'}
								<select
									id="q-default-{i}"
									class="text-input-filled"
									bind:value={question.default}
									disabled={readonly}
								>
									<option value="">(none)</option>
									<option value="true">true</option>
									<option value="false">false</option>
								</select>
							{:else}
								<input
									id="q-default-{i}"
									bind:value={question.default}
									class="text-input-filled"
									placeholder={question.type === 'schedule' ? '0 3 * * *' : ''}
									disabled={readonly}
								/>
							{/if}
						</div>
						<div class="flex items-center gap-4 pb-2">
							<label class="flex items-center gap-2 text-sm font-light">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									bind:checked={question.required}
									disabled={readonly}
								/>
								Required
							</label>
							<label class="flex items-center gap-2 text-sm font-light">
								<input
									type="checkbox"
									class="checkbox checkbox-sm"
									bind:checked={question.sensitive}
									disabled={readonly}
								/>
								Sensitive
							</label>
						</div>
					</div>

					{#if typeHint(question.type)}
						<span class="text-muted-content text-xs">{typeHint(question.type)}</span>
					{/if}

					{#if question.type === 'select'}
						<div class="flex flex-col gap-2">
							<div class="flex items-center justify-between">
								<span class="text-sm font-light">Options</span>
								{#if !readonly}
									<button
										class="btn btn-secondary flex items-center gap-1 text-xs"
										onclick={() => addOption(i)}
									>
										<Plus class="size-3" /> Add Option
									</button>
								{/if}
							</div>
							{#each question.options ?? [] as _, oi (oi)}
								<div class="flex items-center gap-2">
									<input
										bind:value={question.options![oi]}
										class="text-input-filled grow"
										placeholder="Option value"
										disabled={readonly}
										aria-label="Option {oi + 1}"
									/>
									{#if !readonly}
										<IconButton
											variant="danger"
											onclick={() => removeOption(i, oi)}
											tooltip={{ text: 'Remove Option' }}
										>
											<Trash2 class="size-4" />
										</IconButton>
									{/if}
								</div>
							{/each}
							{#if (question.options ?? []).length === 0}
								<p class="text-muted-content text-xs">A choice needs at least one option.</p>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>
