<script lang="ts">
	import Complete from '../Complete.svelte';

	type Props = {
		data: {
			redirectURL: string;
		};
	};

	let { data }: Props = $props();
</script>

<Complete redirectURL={data.redirectURL} />
