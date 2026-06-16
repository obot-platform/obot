<script lang="ts">
	import { appPreferences } from '$lib/stores';

	let logoSrc = $derived(appPreferences.current.logos?.logoIcon);
</script>

<div
	class="size-5"
	role="img"
	aria-label="Obot logo icon"
	style="background-color: currentColor; mask: url({logoSrc}) center / contain no-repeat; -webkit-mask: url({logoSrc}) center / contain no-repeat;"
></div>
