<script lang="ts">
</script>

<div class="flex h-dvh flex-col items-center justify-center gap-2 p-5">
	<h2 class="self-center text-xl">Buttons</h2>
	<div class="flex gap-2">
		<button class="btn">button</button>
		<button class="btn btn-secondary">btn-secondary</button>
		<button class="btn btn-primary">btn-primary</button>
	</div>

	<div class="mt-5 flex flex-col gap-3">
		<h2 class="self-center text-xl">Surfaces</h2>
		<div class="colors grid grid-cols-4 gap-4">
			<div class="bg-base-100 text-base-content">
				<span>background</span>
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
		border-color: var(--color-base-content);
	}
</style>
