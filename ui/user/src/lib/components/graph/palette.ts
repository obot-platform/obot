/**
 * Shared chart palette.
 *
 * Categorical slots are assigned in fixed order (never cycled) and were validated for
 * colorblind separation against the app's light (`base-100`) and dark (`base-200`) paper
 * surfaces. Dark mode uses the same hues stepped for the dark surface. Past the last slot,
 * series fold into the neutral "other" color.
 */
export const CATEGORICAL_LIGHT = [
	'#2a78d6', // blue
	'#eb6834', // orange
	'#1baf7a', // aqua
	'#eda100', // yellow
	'#e87ba4', // magenta
	'#008300', // green
	'#6250d6', // violet
	'#e34948' // red
] as const;

export const CATEGORICAL_DARK = [
	'#3987e5',
	'#d95926',
	'#199e70',
	'#c98500',
	'#d55181',
	'#008300',
	'#9085e9',
	'#e66767'
] as const;

export const OTHER_LIGHT = '#c3c2b7';
export const OTHER_DARK = '#52514e';

export function categoricalPalette(isDark: boolean): readonly string[] {
	return isDark ? CATEGORICAL_DARK : CATEGORICAL_LIGHT;
}

export function otherColor(isDark: boolean): string {
	return isDark ? OTHER_DARK : OTHER_LIGHT;
}

/** Color for the i-th category; anything past the palette folds into "other". */
export function categoricalColor(i: number, isDark: boolean): string {
	return categoricalPalette(isDark)[i] ?? otherColor(isDark);
}
