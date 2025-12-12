import { createContext } from 'svelte';

type Breakpoints = {
	xs: number;
	sm: number;
	md: number;
	lg: number;
	xl: number;
	'2xl': number;
};

const [get, set] = createContext<Viewport>();

export class Viewport {
	#defaultBreakpoints: Breakpoints = {
		xs: 0,
		sm: 576,
		md: 768,
		lg: 1024,
		xl: 1280,
		'2xl': 1536
	};

	#browserBreakpoints: Breakpoints | undefined = $state();

	#width = $state(0);
	#height = $state(0);

	constructor() {
		$effect(() => {
			this.#browserBreakpoints = initializeBreakpoints();

			const resizeHandler = () => {
				this.#width = window.visualViewport?.width ?? 0;
				this.#height = window.visualViewport?.height ?? 0;
			};

			window.addEventListener('resize', resizeHandler);

			resizeHandler();

			return () => {
				window.removeEventListener('resize', resizeHandler);
			};
		});
	}

	get breakpoints() {
		return {
			...this.#defaultBreakpoints,
			...this.#browserBreakpoints
		};
	}

	get width() {
		return this.#width;
	}

	get height() {
		return this.#height;
	}

	// return which screen breakpoint the viewport is in, xs, sm, md, lg, xl
	get screen() {
		const { sm, md, lg, xl, '2xl': xxl } = this.breakpoints;

		return this.width < sm
			? 'xs'
			: this.width < md
				? 'sm'
				: this.width < lg
					? 'md'
					: this.width < xl
						? 'lg'
						: this.width < xxl
							? 'xl'
							: '2xl';
	}

	share() {
		return set(this);
	}

	static get = get;
	static set = set;
}

function initializeBreakpoints() {
	if (typeof document === 'undefined') return undefined;

	const cumputedStyle = getComputedStyle(document.documentElement);

	const getBreakpoint = (varName: string, fallback: number): number => {
		const value = cumputedStyle.getPropertyValue(varName).trim();

		if (!value) return fallback;

		if (value.endsWith('rem')) {
			const remValue = parseFloat(value);
			const rootFontSize = parseFloat(cumputedStyle.fontSize);

			return remValue * rootFontSize;
		}

		return parseInt(value);
	};

	const sm = getBreakpoint('--breakpoint-sm', 576);
	const md = getBreakpoint('--breakpoint-md', 768);
	const lg = getBreakpoint('--breakpoint-lg', 1024);
	const xl = getBreakpoint('--breakpoint-xl', 1280);
	const xxl = getBreakpoint('--breakpoint-2xl', 1536);

	return {
		sm,
		md,
		lg,
		xl,
		'2xl': xxl
	} as Breakpoints;
}
