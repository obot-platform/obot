<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import { goto } from '$lib/url';
	import GuardrailForm from './GuardrailForm.svelte';

	// A guardrail's page, under LLM Gateway; going back returns to its Guardrails tab.
	let routeID = $derived(page.params.id ?? 'new');
	let title = $state('');

	function back() {
		goto('/models?view=guardrails');
	}
</script>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

<Layout {title} showBackButton onBackButtonClick={back}>
	<GuardrailForm
		id={routeID === 'new' ? undefined : routeID}
		type={page.url.searchParams.get('type') ?? undefined}
		onTitle={(t) => (title = t)}
		onSaved={(saved, created) => {
			// The page stays open after creating, so the new guardrail can be tested.
			if (created && saved.id) {
				goto(`/models/guardrails/${saved.id}`, { replaceState: true });
			}
		}}
		onCancel={back}
	/>
</Layout>
