<script lang="ts">
	import type { MCPCatalogEntryFieldManifest } from '$lib/services';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Select from '../Select.svelte';
	import IconButton from '../primitives/IconButton.svelte';
	import {
		actionHelp,
		actionLabels,
		credentialActions,
		credentialCatalog,
		credentialDefaultKey,
		credentialListKeys,
		readCredentialPolicy,
		writeCredentialPolicy,
		validateCredentialPolicy,
		type CredentialAction,
		type CredentialPolicy
	} from './credentialPolicy';
	import { Plus, Trash2 } from '@lucide/svelte';

	let {
		config = $bindable(),
		readonly = false
	}: { config: MCPCatalogEntryFieldManifest[]; readonly?: boolean } = $props();
	const actionOptions = credentialActions.map((id) => ({ id, label: actionLabels[id] }));
	const selectClass =
		'bg-base-200 shadow-inner! dark:bg-base-100 dark:border-base-400 border border-transparent';
	let policy = $derived(readCredentialPolicy(config));
	let error = $derived(validateCredentialPolicy(policy));
	let picker = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let selected = $state('');
	let action = $state<CredentialAction>('block');
	let matches = $derived(
		credentialCatalog.rules.filter((rule) =>
			`${rule.provider} ${rule.name} ${rule.id}`.toLowerCase().includes(search.trim().toLowerCase())
		)
	);
	let providers = $derived([...new Set(matches.map((rule) => rule.provider))]);
	function update(next: CredentialPolicy) {
		config = writeCredentialPolicy(config, next);
	}
	function remove(index: number) {
		// Permit repairing invalid saved policies without dropping other assignments.
		const override = policy.overrides[index];
		const key = credentialListKeys[override.action];
		let removed = false;
		config = config.map((field) =>
			field.key !== key
				? field
				: {
						...field,
						value: (field.value || '')
							.split(',')
							.map((id) => id.trim())
							.filter((id) => {
								if (!removed && id === override.ruleId) {
									removed = true;
									return false;
								}
								return Boolean(id);
							})
							.sort()
							.join(',')
					}
		);
	}
</script>

<div
	class="dark:bg-base-200 dark:border-base-400 bg-base-100 flex flex-col gap-4 rounded-lg border border-transparent p-4 shadow-sm"
>
	<h4 class="text-sm font-semibold">Configuration</h4>
	<div class="w-full flex-col md:flex-row flex md:items-start md:justify-between md:gap-4 gap-1">
		<span id="credential-default-label" class="font-light md:pt-2">Default action</span>
		<div class="grid w-full md:w-96 shrink-0 grid-cols-[minmax(0,1fr)_2.5rem] gap-x-4 gap-y-1">
			<Select
				id="credential-default"
				ariaLabelledby="credential-default-label"
				ariaDescribedby="credential-default-help"
				classes={{ root: 'min-w-0 col-start-1' }}
				class={selectClass}
				options={actionOptions}
				selected={policy.defaultAction}
				disabled={readonly}
				onSelect={({ id }) => {
					const defaultField = config.find((field) => field.key === credentialDefaultKey);
					config = defaultField
						? config.map((field) =>
								field.key === credentialDefaultKey ? { ...field, value: id } : field
							)
						: [
								...config,
								{
									key: credentialDefaultKey,
									name: credentialDefaultKey,
									value: id,
									description: '',
									required: false,
									sensitive: false
								}
							];
				}}
			/>
			<p id="credential-default-help" class="col-start-1 text-xs font-light text-muted-content">
				{actionHelp[policy.defaultAction as CredentialAction] || 'Choose a valid default action.'}
			</p>
		</div>
	</div>
	<h4 class="text-sm font-semibold">Credential overrides</h4>
	{#if error}<p role="alert" class="text-error text-sm">{error}</p>{/if}
	{#if policy.overrides.length === 0}<p class="text-sm text-muted-content">
			No overrides. All credential types use the default action.
		</p>{/if}
	<div class="flex flex-col md:gap-2 gap-4 w-full">
		{#each policy.overrides as override, index (index)}
			{@const rule = credentialCatalog.rules.find((rule) => rule.id === override.ruleId)}
			<div
				class="w-full flex-col md:flex-row flex md:items-center md:justify-between md:gap-4 gap-1"
			>
				<div class="min-w-0 grow">
					<span id={`credential-override-${index}-label`} class="font-light"
						>{rule?.name || override.ruleId}</span
					>
					<p class="text-xs font-light text-muted-content break-words">
						{rule?.provider || 'Unsupported rule'} / {override.ruleId}
					</p>
				</div>
				<div class="flex w-full md:w-96 shrink-0 items-center gap-4">
					<Select
						id={`credential-override-${index}`}
						ariaLabelledby={`credential-override-${index}-label`}
						classes={{ root: 'min-w-0 flex-1' }}
						class={selectClass}
						options={actionOptions}
						selected={override.action}
						disabled={readonly || !!error}
						onSelect={({ id }) =>
							update({
								...policy,
								overrides: policy.overrides.map((item, i) =>
									i === index ? { ...item, action: id } : item
								)
							})}
					/>
					<IconButton
						variant="danger"
						disabled={readonly}
						aria-label={`Remove ${rule?.name || override.ruleId}`}
						onclick={() => remove(index)}><Trash2 class="size-4" /></IconButton
					>
				</div>
			</div>
		{/each}
	</div>
	<button
		type="button"
		class="btn btn-secondary flex items-center gap-1 btn-sm self-end"
		disabled={readonly || !!error}
		onclick={() => {
			search = '';
			selected = '';
			action = policy.defaultAction as CredentialAction;
			picker?.open();
		}}
		aria-label="+ Add override"><Plus class="size-4" /> Add override</button
	>
	<p class="text-sm text-muted-content">
		Any blocking match rejects the entire message, including matches from other rules on the same
		credential.
	</p>
	<details class="text-sm text-muted-content">
		<summary>Detection and redaction limits</summary>
		<p class="mt-2">
			Detection is pattern-based, can produce false positives, and does not recognize every secret.
			Credentials are not validated. Redaction uses [REDACTED_CREDENTIAL] and may replace
			surrounding context or an entire field value. Redact findings in object keys or JSON-RPC
			identifiers block the message instead. Allow passes credentials unchanged and does not create
			a findings history.
		</p>
		<p class="mt-2">
			Select both request and response directions where both need protection. We recommend
			configuring the gateway to reject requests on filter transport or execution failures.
		</p>
	</details>
</div>
<ResponsiveDialog bind:this={picker} title="Add credential override">
	<div class="flex flex-col gap-4 p-4 md:p-0">
		<label for="credential-search">Search provider, credential type, or rule ID</label>
		<input
			id="credential-search"
			class="text-input-filled"
			bind:value={search}
			oninput={() => (selected = '')}
			placeholder="Search provider, credential type, or rule ID..."
		/>
		<div class="max-h-72 overflow-y-auto flex flex-col gap-3">
			{#if !matches.length}<p>No credential types found</p>{/if}
			{#each providers as provider (provider)}
				<fieldset>
					<legend class="font-semibold">{provider}</legend>
					{#each matches.filter((rule) => rule.provider === provider) as rule (rule.id)}
						{@const added = policy.overrides.some((override) => override.ruleId === rule.id)}
						<label class="flex items-start gap-2 py-2"
							><input
								type="radio"
								name="credential-rule"
								value={rule.id}
								bind:group={selected}
								disabled={added || readonly}
							/><span
								>{rule.name}<span class="block text-sm text-muted-content"
									>{rule.id}{added ? ' — Already added' : ''}</span
								></span
							></label
						>
					{/each}
				</fieldset>
			{/each}
		</div>
		<div class="w-full flex-col md:flex-row flex md:items-center md:justify-between md:gap-4 gap-1">
			<span id="credential-action-label" class="font-light">Action</span>
			<Select
				id="credential-action"
				ariaLabelledby="credential-action-label"
				classes={{ root: 'w-full md:w-96' }}
				class={selectClass}
				options={actionOptions}
				selected={action}
				disabled={readonly}
				onSelect={({ id }) => {
					action = id;
				}}
			/>
		</div>
		<p class="text-sm text-muted-content">
			{action === 'allow'
				? 'Detected credentials matching this rule pass through unchanged. Another matching rule can still block the message.'
				: actionHelp[action]}
		</p>
		<div class="flex justify-end gap-2">
			<button type="button" class="btn btn-secondary" onclick={() => picker?.close()}>Cancel</button
			><button
				type="button"
				class="btn btn-primary"
				disabled={readonly ||
					!selected ||
					!action ||
					policy.overrides.some((override) => override.ruleId === selected)}
				onclick={() => {
					update({ ...policy, overrides: [...policy.overrides, { ruleId: selected, action }] });
					picker?.close();
				}}>Add override</button
			>
		</div>
	</div>
</ResponsiveDialog>
