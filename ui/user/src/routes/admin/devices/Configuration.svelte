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
	/>
{:else}
	<GettingStarted readOnly={isAdminReadonly} onCreate={handleCreate} />
{/if}
