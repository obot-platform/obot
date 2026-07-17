<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import Select from '../Select.svelte';

	interface Props {
		field: MCPSubField;
		targets: MCPAllowedSecretBindingTarget[];
		readonly?: boolean;
	}

	let { field = $bindable(), targets, readonly }: Props = $props();
	type SecretBindingOption = { id: string; label: string; disabled?: boolean };
	type ValueSource = 'value' | 'secret';
	type SecretBindingField = MCPSubField & { secretBindingSource?: ValueSource };
	let valueSource = $state<ValueSource>(field.secretBinding ? 'secret' : 'value');

	$effect(() => {
		(field as SecretBindingField).secretBindingSource = valueSource;
	});

	// Pinned/template-owned bindings (secretBindingReadonly) are owned by the catalog
	// entry; the backend rejects overrides, so never offer edits for them here.
	const isReadonly = $derived(
		readonly || Boolean((field as { secretBindingReadonly?: boolean }).secretBindingReadonly)
	);

	const sourceOptions = $derived([
		{ id: 'value', label: 'Manual Value' },
		{ id: 'secret', label: 'Kubernetes Secret' }
	]);
	const secretOptions = $derived.by(() => {
		const options: SecretBindingOption[] = targets.map((target) => ({
			id: target.name,
			label: target.name
		}));
		const boundSecret = field.secretBinding?.name;

		if (boundSecret && !targets.some((target) => target.name === boundSecret)) {
			options.push({ id: boundSecret, label: `${boundSecret} (not available)`, disabled: true });
		}

		return options;
	});
	const selectedTarget = $derived(
		targets.find((target) => target.name === field.secretBinding?.name)
	);
	const keyOptions = $derived.by(() => {
		const options: SecretBindingOption[] = (selectedTarget?.keys ?? []).map((key) => ({
			id: key,
			label: key
		}));
		const boundKey = field.secretBinding?.key;

		if (boundKey && !options.some((option) => option.id === boundKey)) {
			options.push({ id: boundKey, label: `${boundKey} (not available)`, disabled: true });
		}

		return options;
	});
	const selectClasses =
		'bg-base-200 dark:bg-base-100 border border-base-300 dark:border-base-400 w-full shadow-inner';

	function setValueSource(source: ValueSource) {
		valueSource = source;
		(field as SecretBindingField).secretBindingSource = source;
	}

	function enableSecretBinding() {
		setValueSource('secret');
		const firstTarget = targets[0];
		const firstKey = firstTarget?.keys[0];
		if (!firstTarget || !firstKey) return;
		field.value = '';
		field.secretBinding = { name: firstTarget.name, key: firstKey };
		field.sensitive = true;
	}

	function clearSecretBinding() {
		setValueSource('value');
		field.secretBinding = undefined;
	}

	function selectSecret(name: string | number) {
		setValueSource('secret');
		const target = targets.find((candidate) => candidate.name === String(name));
		const key = target?.keys[0];
		if (!target || !key) return;
		field.value = '';
		field.secretBinding = { name: target.name, key };
		field.sensitive = true;
	}

	function selectKey(key: string | number) {
		setValueSource('secret');
		if (!field.secretBinding) return;
		field.value = '';
		field.secretBinding = { ...field.secretBinding, key: String(key) };
		field.sensitive = true;
	}
</script>

<div class="flex w-full flex-col gap-3">
	<div class="flex w-full flex-col gap-1">
		<label for={`secret-binding-source-${field.key}`} class="text-sm font-light">Value Source</label
		>
		<Select
			id={`secret-binding-source-${field.key}`}
			class={selectClasses}
			options={sourceOptions}
			selected={valueSource}
			disabled={isReadonly}
			onSelect={(option) => {
				if (option.id === 'secret') {
					enableSecretBinding();
				} else {
					clearSecretBinding();
				}
			}}
		/>
	</div>

	{#if valueSource === 'secret'}
		<div class="grid gap-3 md:grid-cols-2">
			<div class="flex w-full flex-col gap-1">
				<label for={`secret-binding-secret-${field.key}`} class="text-sm font-light">Secret</label>
				<Select
					id={`secret-binding-secret-${field.key}`}
					class={selectClasses}
					options={secretOptions}
					selected={field.secretBinding?.name}
					disabled={isReadonly || targets.length === 0}
					placeholder="No secrets found"
					searchInDropdown
					onSelect={(option) => selectSecret(option.id)}
				/>
			</div>
			<div class="flex w-full flex-col gap-1">
				<label for={`secret-binding-key-${field.key}`} class="text-sm font-light">Key</label>
				<Select
					id={`secret-binding-key-${field.key}`}
					class={selectClasses}
					options={keyOptions}
					selected={field.secretBinding?.key}
					disabled={isReadonly || keyOptions.length === 0}
					placeholder="N/A"
					searchInDropdown
					onSelect={(option) => selectKey(option.id)}
				/>
			</div>
		</div>
	{/if}
</div>
