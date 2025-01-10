import scrollbars from 'tailwind-scrollbar';
import type { Config } from 'tailwindcss';
import colors from 'tailwindcss/colors';

const grayBase = 2.5;

export default {
	content: [
		'./src/**/*.{html,js,svelte,ts}',
		'./node_modules/flowbite-svelte/**/*.{html,js,svelte,ts}'
	],

	darkMode: 'class',

	plugins: [scrollbars],

	theme: {
		fontFamily: {
			sans: [
				'Poppins',
				'ui-sans-serif',
				'system-ui',
				'-apple-system',
				'system-ui',
				'Segoe UI',
				'Roboto',
				'Helvetica Neue',
				'Arial',
				'Noto Sans',
				'sans-serif',
				'Apple Color Emoji',
				'Segoe UI Emoji',
				'Segoe UI Symbol',
				'Noto Color Emoji'
			],
			body: [
				'Poppins',
				'ui-sans-serif',
				'system-ui',
				'-apple-system',
				'system-ui',
				'Segoe UI',
				'Roboto',
				'Helvetica Neue',
				'Arial',
				'Noto Sans',
				'sans-serif',
				'Apple Color Emoji',
				'Segoe UI Emoji',
				'Segoe UI Symbol',
				'Noto Color Emoji'
			],
			mono: [
				'ui-monospace',
				'SFMono-Regular',
				'Menlo',
				'Monaco',
				'Consolas',
				'Liberation Mono',
				'Courier New',
				'monospace'
			]
		},

		colors: {
			transparent: 'transparent',
			current: 'currentColor',
			white: colors.white,
			black: colors.black,
			red: colors.red,
			gray: {
				DEFAULT: `hsl(0, 0, ${grayBase + 50})`,
				'50': `hsl(0, 0, ${grayBase + 95})`,
				'60': `hsl(0, 0, ${grayBase + 94})`,
				'70': `hsl(0, 0, ${grayBase + 93})`,
				'80': `hsl(0, 0, ${grayBase + 92})`,
				'90': `hsl(0, 0, ${grayBase + 91})`,
				'100': `hsl(0, 0, ${grayBase + 90})`,
				'200': `hsl(0, 0, ${grayBase + 80})`,
				'300': `hsl(0, 0, ${grayBase + 70})`,
				'400': `hsl(0, 0, ${grayBase + 60})`,
				'500': `hsl(0, 0, ${grayBase + 50})`,
				'600': `hsl(0, 0, ${grayBase + 40})`,
				'700': `hsl(0, 0, ${grayBase + 30})`,
				'800': `hsl(0, 0, ${grayBase + 20})`,
				'900': `hsl(0, 0, ${grayBase + 10})`,
				'910': `hsl(0, 0, ${grayBase + 9})`,
				'920': `hsl(0, 0, ${grayBase + 8})`,
				'930': `hsl(0, 0, ${grayBase + 7})`,
				'940': `hsl(0, 0, ${grayBase + 6})`,
				'950': `hsl(0, 0, ${grayBase + 5})`,
				'960': `hsl(0, 0, ${grayBase + 4})`,
				'970': `hsl(0, 0, ${grayBase + 3})`,
				'980': `hsl(0, 0, ${grayBase + 2})`,
				'990': `hsl(0, 0, ${grayBase + 1})`
			},
			blue: {
				DEFAULT: '#4f7ef3',
				'50': '#eff5ff',
				'100': '#dce7fd',
				'200': '#c0d5fd',
				'300': '#95bcfb',
				'400': '#6397f7',
				'500': '#4f7ef3',
				'600': '#2953e7',
				'700': '#213fd4',
				'800': '#2135ac',
				'900': '#203188',
				'950': '#182153'
			}
		}
	}
} as Config;
