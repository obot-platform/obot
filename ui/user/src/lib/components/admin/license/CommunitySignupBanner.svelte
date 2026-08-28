<script lang="ts">
	import { resolve } from '$app/paths';
	import { COMMUNITY_SIGNUP_BANNER_COPY } from '$lib/constants';
	import { X } from '@lucide/svelte';

	interface Props {
		onDismiss: () => void;
	}

	let { onDismiss }: Props = $props();
</script>

<div class="bg-base-100 w-full min-h-8.5">
	<div
		class="bg-primary/10 text-primary w-full py-2 px-4 flex items-center justify-center gap-2"
		role="status"
	>
		<p class="text-xs font-light max-w-2xl">
			{COMMUNITY_SIGNUP_BANNER_COPY}
		</p>
		<a href={resolve('/admin/license')} class="btn btn-xs btn-primary shrink-0"> Register </a>
		<button
			class="btn btn-circle text-primary hover:text-base-content btn-xs w-fit h-fit p-0.5 dark:hover:bg-base-400"
			onclick={onDismiss}
			type="button"
			aria-label="Dismiss community signup banner"
		>
			<X class="size-3" />
		</button>
	</div>
</div>
