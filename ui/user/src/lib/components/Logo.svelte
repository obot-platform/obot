<script lang="ts">
	import appPreferences from '$lib/stores/appPreferences.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		class?: string;
		variant?: 'blue' | 'error' | 'warning';
	}

	const logos = $derived({
		blue: appPreferences.current.logos?.logoIcon,
		error: appPreferences.current.logos?.logoIconError,
		warning: appPreferences.current.logos?.logoIconWarning
	});

	let { class: klass, variant = 'blue' }: Props = $props();
	const classes = $derived(twMerge('h-8', klass));
	const logoSrc = $derived(logos[variant]);
</script>

<img src={logoSrc} class={classes} alt="Obot {variant} logo icon" />
