<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import Select from '../Select.svelte';

	interface Props {
		field: MCPSubField;
		targets: MCPAllowedSecretBindingTarget[];
		readonly?: boolean;
	}

	let { field, targets, readonly }: Props = $props();
	type SecretBindingOption = { id: string; label: string; disabled?: boolean };

	const sourceOptions = $derived([
		{ id: 'value', label: 'Manual Value' },
		{ id: 'secret', label: 'Kubernetes Secret', disabled: targets.length === 0 }
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
		'bg-base-200 dark:bg-base-300 border border-base-300 dark:border-base-400 w-full shadow-inner';

	function enableSecretBinding() {
		const firstTarget = targets[0];
		const firstKey = firstTarget?.keys[0];
		if (!firstTarget || !firstKey) return;
		field.value = '';
		field.secretBinding = { name: firstTarget.name, key: firstKey };
	}

	function clearSecretBinding() {
		field.secretBinding = undefined;
	}

	function selectSecret(name: string | number) {
		const target = targets.find((candidate) => candidate.name === String(name));
		const key = target?.keys[0];
		if (!target || !key) return;
		field.value = '';
		field.secretBinding = { name: target.name, key };
	}

	function selectKey(key: string | number) {
		if (!field.secretBinding) return;
		field.value = '';
		field.secretBinding = { ...field.secretBinding, key: String(key) };
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
			selected={field.secretBinding ? 'secret' : 'value'}
			disabled={readonly}
			onSelect={(option) => {
				if (option.id === 'secret') {
					enableSecretBinding();
				} else {
					clearSecretBinding();
				}
			}}
		/>
		{#if targets.length === 0}
			<p class="text-muted-content text-xs font-light">
				No labeled Kubernetes Secrets are available for binding.
			</p>
		{/if}
	</div>

	{#if field.secretBinding}
		<div class="grid gap-3 md:grid-cols-2">
			<div class="flex w-full flex-col gap-1">
				<label for={`secret-binding-secret-${field.key}`} class="text-sm font-light">Secret</label>
				<Select
					id={`secret-binding-secret-${field.key}`}
					class={selectClasses}
					options={secretOptions}
					selected={field.secretBinding.name}
					disabled={readonly}
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
					selected={field.secretBinding.key}
					disabled={readonly || keyOptions.length === 0}
					searchInDropdown
					onSelect={(option) => selectKey(option.id)}
				/>
			</div>
		</div>
	{/if}
</div>
