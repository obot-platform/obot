<script lang="ts">
	import {
		type MDMAsset,
		type MDMAssetSource,
		type MDMConfiguration,
		type MDMEnrollmentKey
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import ConfigurationDetails from './ConfigurationDetails.svelte';
	import GettingStarted from './GettingStarted.svelte';
	import { untrack } from 'svelte';

	interface Props {
		configuration?: MDMConfiguration;
		enrollmentKeys: MDMEnrollmentKey[];
		assetSource?: MDMAssetSource;
		assets: MDMAsset[];
		assetLoadError?: string;
	}

	let {
		configuration: initialConfiguration,
		enrollmentKeys: initialEnrollmentKeys,
		assetSource,
		assets,
		assetLoadError
	}: Props = $props();

	let configuration = $state<MDMConfiguration | undefined>(untrack(() => initialConfiguration));
	let enrollmentKeys = $state<MDMEnrollmentKey[]>(untrack(() => initialEnrollmentKeys));
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());

	// The copy taken above is owned locally so the create flow can install the
	// configuration it just made, but the page reloads its data whenever the policy
	// is rewritten elsewhere. Adopt what it hands down rather than sitting on a
	// snapshot from mount.
	$effect(() => {
		if (initialConfiguration) {
			configuration = initialConfiguration;
		}
	});

	function handleCreate(created: MDMConfiguration) {
		configuration = created;
		enrollmentKeys = [];
	}
</script>

{#if configuration}
	<ConfigurationDetails
		{configuration}
		{enrollmentKeys}
		{assetSource}
		{assets}
		{assetLoadError}
		readOnly={isAdminReadonly}
		onConfigurationUpdate={(updated) => (configuration = updated)}
	/>
{:else}
	<GettingStarted readOnly={isAdminReadonly} onCreate={handleCreate} />
{/if}
