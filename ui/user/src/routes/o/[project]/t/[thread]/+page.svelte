<script lang="ts">
	import { goto } from '$app/navigation';
	import { type PageProps } from './$types';
	import { browser } from '$app/environment';

	let { data }: PageProps = $props();

	if (browser) {
		goto(`/o/${data.project}`, { replaceState: true, state: { currentThreadID: data.thread } });
	}
</script>
