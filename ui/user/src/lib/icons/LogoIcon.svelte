<script lang="ts">
	import { m } from '$lib/i18n';
	import { appPreferences } from '$lib/stores';

	let logoSrc = $derived(appPreferences.current.logos?.logoIcon);
</script>

<div
	class="size-5"
	role="img"
	aria-label={m.core_obot_logo_icon()}
	style="background-color: currentColor; mask: url({logoSrc}) center / contain no-repeat; -webkit-mask: url({logoSrc}) center / contain no-repeat;"
></div>
