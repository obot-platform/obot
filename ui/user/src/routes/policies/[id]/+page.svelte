<script lang="ts">
	import { page } from '$app/state';
	import Layout from '$lib/components/Layout.svelte';
	import PolicyForm from '$lib/components/supervision/PolicyForm.svelte';
	import { m } from '$lib/i18n';
	import { goto } from '$lib/url';

	// A policy's editor; `new` creates one. Going back returns to the list.
	let routeID = $derived(page.params.id ?? 'new');
	let title = $state<string>(m.supervision_policy_new_title());

	function back() {
		goto('/policies');
	}
</script>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>

<Layout {title} showBackButton onBackButtonClick={back}>
	{#key routeID}
		<PolicyForm
			id={routeID === 'new' ? undefined : routeID}
			startWithDraft={page.url.searchParams.get('draft') === '1'}
			onTitle={(t) => (title = t)}
			onSaved={(saved) => {
				if (saved.id) goto(`/policies/${saved.id}`, { replaceState: true });
				else back();
			}}
			onCancel={back}
		/>
	{/key}
</Layout>
