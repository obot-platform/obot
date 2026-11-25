<script lang="ts">
	import { page } from '$app/state';
	import { m } from '$lib/i18n';
	import { goto } from '$lib/url';

	interface Props {
		// The URL parameter that holds the choice.
		param?: string;
	}

	let { param = 'include_on_behalf_of' }: Props = $props();

	const checked = $derived(page.url.searchParams.get(param) === 'true');

	function change(on: boolean) {
		const url = new URL(page.url.toString());
		if (on) {
			url.searchParams.set(param, 'true');
		} else {
			url.searchParams.delete(param);
		}
		goto(url, { noScroll: true });
	}
</script>

<!-- Listed with a user filter: also lists what agents did on behalf of the user. -->
<label class="flex items-center gap-2 text-sm">
	<input
		type="checkbox"
		class="checkbox checkbox-xs"
		{checked}
		onchange={(e) => change(e.currentTarget.checked)}
	/>
	{m.identity_access_include_on_behalf_of()}
</label>
