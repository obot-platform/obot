<script lang="ts">
	import { Trash2 } from 'lucide-svelte';
</script>

<div class="flex h-screen flex-col items-center justify-center gap-2 p-5">
	<h2 class="self-center text-xl">Buttons</h2>
	<div class="flex gap-2">
		<button class="button">button</button>
		<button class="button-secondary">button-secondary</button>
		<button class="button-primary">button-primary</button>
	</div>

	<h2 class="mt-5 self-center text-xl">Icons</h2>
	<div class="flex items-center gap-2">
		<Trash2 class="icon-default" />
		icon-default
	</div>

	<div class="flex items-center gap-2">
		<button class="icon-button">
			<Trash2 class="icon-default" />
		</button>
		icon-button
	</div>

	<div class="mt-5 flex flex-col gap-3">
		<h2 class="self-center text-xl">Surfaces</h2>
		<div class="colors grid grid-cols-4 gap-4">
			<div class="bg-background text-on-background">
				<span>background</span>
			</div>
			<div class="bg-surface1 text-on-surface1">
				<span>surface1</span>
			</div>
			<div class="bg-surface2 text-on-surface2">
				<span>surface2</span>
			</div>
			<div class="bg-surface3 text-on-surface3">
				<span>surface3</span>
			</div>
		</div>
	</div>

	<button
		class="button mt-10"
		onclick={() => {
			document.documentElement.classList.toggle('dark');
			document.documentElement.classList.toggle('light');
		}}
		>Toggle Light/Dark
	</button>
</div>

<style lang="postcss">
	.colors > div {
		display: flex;
		height: 9rem; /* size-36 */
		width: 9rem; /* size-36 */
		align-items: center;
		justify-content: center;
		border-width: 2px;
		border-style: dotted;
		border-color: var(--on-background);
	}
</style>
