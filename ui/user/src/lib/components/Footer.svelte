<footer class="flex w-full flex-col justify-between text-sm">
	<div class="flex w-full flex-col items-center gap-3 px-5 py-4 md:flex-row">
		<div class="flex grow"></div>
		<div class="mt-8 flex flex-col gap-2 md:mt-0">
			<p class="self-center text-[11px] md:self-end">© 2025 Acorn. All rights reserved.</p>
			<div class="flex gap-4">
				<a href="/privacy-policy">Privacy Policy</a> |
				<a href="/terms-of-service">Terms of Service</a>
			</div>
		</div>
	</div>
</footer>

<style lang="postcss">
	:global {
		.line {
			position: relative;
			z-index: 1;
			&::after {
				content: '';
				position: absolute;
				top: 10px;
				z-index: -1;
				left: 50%;
				transform: translate(-50%, -50%);
				width: 100%;
				height: 1px;
				background-color: #000;
			}
			.dark & {
				&::after {
					background-color: #fff;
				}
			}
		}
	}
</style>
