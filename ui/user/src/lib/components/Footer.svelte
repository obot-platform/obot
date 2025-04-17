<footer class="flex w-full flex-col justify-between text-sm shadow-md">
	<div class="flex w-full flex-col items-center gap-3 px-5 py-4 md:flex-row">
		<a href="https://www.acorn.io/">
			<img
				src="/user/images/acorn-mark/acorn-mark-teal.svg"
				class="size-12"
				alt="acorn labs logo"
			/>
		</a>
		<div class="line flex flex-col items-center gap-1">
			<p class="bg-white px-2 font-light dark:bg-black">Contact Us</p>
			<div class="flex items-center gap-2">
				<a href="/">
					<img
						src="/user/images/social/twitter-mark.svg"
						class="size-6"
						alt="twitter social logo"
					/>
				</a>
				<a href="/">
					<img
						src="/user/images/social/linkedin-mark.svg"
						class="size-6"
						alt="linkedin social logo"
					/>
				</a>
				<a href="/">
					<img
						src="/user/images/social/facebook-mark.svg"
						class="size-6"
						alt="facebook social logo"
					/>
				</a>
				<a href="/">
					<img
						src="/user/images/social/youtube-mark.svg"
						class="size-6"
						alt="youtube social logo"
					/>
				</a>
				<a href="/">
					<img
						src="/user/images/social/instagram-mark.svg"
						class="size-6"
						alt="instagram social logo"
					/>
				</a>
			</div>
		</div>
		<div class="flex grow"></div>
		<div class="mt-8 flex flex-col gap-2 md:mt-0">
			<p class="self-center text-[11px] md:self-end">© 2025 Acorn. All rights reserved.</p>
			<div class="flex gap-4">
				<a href="/">Privacy Policy</a> |
				<a href="/">Terms of Service</a>
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
