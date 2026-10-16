<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import {
		listGuardrailProviders,
		listGuardrails,
		type Guardrail,
		type GuardrailProvider
	} from '$lib/services/llm-gateway/guardrails';
	import GuardrailForm from './GuardrailForm.svelte';
	import GuardrailTypePicker from './GuardrailTypePicker.svelte';

	// Creates a guardrail without leaving the form that uses it: pick a type, fill in its
	// settings, and the new guardrail is handed to onCreated to be selected.
	interface Props {
		onCreated: (guardrail: Guardrail) => void;
	}

	let { onCreated }: Props = $props();

	let providers = $state<GuardrailProvider[]>([]);
	let guardrails = $state<Guardrail[]>([]);
	let picker = $state<ReturnType<typeof GuardrailTypePicker>>();
	let formDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let type = $state<string>();
	let title = $state('');
	let formKey = $state(0);
	let error = $state('');

	export async function open() {
		error = '';
		if (providers.length === 0) {
			try {
				providers = await listGuardrailProviders();
			} catch (e) {
				error = parseErrorContent(e).message;
				return;
			}
		}
		// Only marks the integrations that are already configured; the picker works without it.
		guardrails = await listGuardrails().catch((): Guardrail[] => []);
		picker?.open();
	}

	function close() {
		formDialog?.close();
		type = undefined;
	}
</script>

{#if error}
	<p class="text-error text-xs">{error}</p>
{/if}

<GuardrailTypePicker
	bind:this={picker}
	{providers}
	{guardrails}
	onSelect={(p) => {
		type = p.type;
		formKey++;
		formDialog?.open();
	}}
/>

<ResponsiveDialog
	bind:this={formDialog}
	title={title || m.guardrails_new_title_generic()}
	class="max-w-3xl"
	onClose={() => (type = undefined)}
>
	<div class="max-h-[75vh] overflow-y-auto pt-4">
		{#if type}
			{#key formKey}
				<GuardrailForm
					{type}
					embedded
					onTitle={(t) => (title = t)}
					onSaved={(guardrail) => {
						close();
						onCreated(guardrail);
					}}
					onCancel={close}
				/>
			{/key}
		{/if}
	</div>
</ResponsiveDialog>
