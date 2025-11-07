<script lang="ts">
	import { twMerge } from 'tailwind-merge';
	import { LOGO_CONFIG } from '$lib/config/logos';

	interface Props {
		class?: string;
		variant?: 'blue' | 'white' | 'error' | 'warning';
	}

	let { class: klass, variant = 'blue' }: Props = $props();
	const classes = $derived(twMerge('h-8', klass));
	const logoSrc = $derived(LOGO_CONFIG.icon[variant]);
</script>

<img src={logoSrc} class={classes} alt="Obot {variant} logo icon" />
