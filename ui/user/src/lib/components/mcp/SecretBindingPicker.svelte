<script lang="ts">
	import type { MCPAllowedSecretBindingTarget, MCPSubField } from '$lib/services';
	import Select from '../Select.svelte';

	interface Props {
		field: MCPSubField;
		targets: MCPAllowedSecretBindingTarget[];
		readonly?: boolean;
	}

	let { field, targets, readonly }: Props = $props();

	const sourceOptions = $derived([
		{ id: 'value', label: 'Manual Value' },
		{ id: 'secret', label: 'Kubernetes Secret', disabled: targets.length === 0 }
	]);
	const secretOptions = $derived(
		targets.map((target) => ({ id: target.name, label: target.name }))
	);
	const selectedTarget = $derived(
		targets.find((target) => target.name === field.secretBinding?.name)
	);
	const keyOptions = $derived((selectedTarget?.keys ?? []).map((key) => ({ id: key, label: key })));

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
			class="text-input-filled bg-base-100 w-full shadow-none"
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
					class="text-input-filled bg-base-100 w-full shadow-none"
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
					class="text-input-filled bg-base-100 w-full shadow-none"
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
