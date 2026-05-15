export function lightenHex(hex: string, amount: number): string {
	const h = hex.replace(/^#/, '');
	const r = parseInt(h.slice(0, 2), 16);
	const g = parseInt(h.slice(2, 4), 16);
	const b = parseInt(h.slice(4, 6), 16);
	const mix = (c: number) => Math.round(c + (255 - c) * amount);
	return `#${[r, g, b].map((c) => mix(c).toString(16).padStart(2, '0')).join('')}`;
}

/**
 * Converts a hue on the ROYGBIV spectrum (full saturation, mid lightness)
 * to sRGB bytes. Hue is in degrees [0, 360].
 */
export function hueSpectrumToRgb(hue: number): { r: number; g: number; b: number } {
	const h = ((hue % 360) + 360) % 360;
	const s = 1;
	const l = 0.5;
	const c = (1 - Math.abs(2 * l - 1)) * s;
	const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
	const m = l - c / 2;
	let rp = 0;
	let gp = 0;
	let bp = 0;
	if (h < 60) {
		rp = c;
		gp = x;
	} else if (h < 120) {
		rp = x;
		gp = c;
	} else if (h < 180) {
		gp = c;
		bp = x;
	} else if (h < 240) {
		gp = x;
		bp = c;
	} else if (h < 300) {
		rp = x;
		bp = c;
	} else {
		rp = c;
		bp = x;
	}
	return {
		r: Math.round((rp + m) * 255),
		g: Math.round((gp + m) * 255),
		b: Math.round((bp + m) * 255)
	};
}

export function rgbToHex(r: number, g: number, b: number): string {
	const clamp = (n: number) => Math.max(0, Math.min(255, n));
	const toHex = (n: number) => clamp(n).toString(16).padStart(2, '0');
	return `#${toHex(r)}${toHex(g)}${toHex(b)}`;
}

/** Max HSL saturation (0–1) applied when tint is 100%. Kept moderate for UI surfaces. */
const TINT_MAX_SATURATION = 0.65;

/** Soft HSL lightness bounds so tone/tint never land on pitch black or pure white. */
const SOFT_L_MIN = 0.035;
const SOFT_L_MAX = 0.972;

/**
 * Max change in HSL lightness (shade 0 vs 100 from center). Shade adjusts underlying tone, not an
 * RGB overlay—whole surface ladder moves darker/lighter together.
 */
const SHADE_LIGHTNESS_DELTA_MAX = 0.19;

/** Scales slider extremes so tone shifts stay gentle for UI. */
const SHADE_TONE_ATTENUATION = 0.82;

/** Discrete shade control: integer ticks with a neutral midpoint (UI slider). */
export const SHADE_TICK_MIN = 0;
export const SHADE_TICK_MAX = 6;
export const SHADE_TICK_NEUTRAL = 3;

/**
 * In HSL, lightness at 0 or 1 collapses all hues to black or white—so a pure white background
 * never shows tint. When applying chroma, clamp into (0, 1) so hue/saturation can surface while
 * keeping relative lightness between background and surfaces.
 */
function clampLightnessForHslChroma(lightness: number): number {
	return Math.min(Math.max(lightness, SOFT_L_MIN), SOFT_L_MAX);
}

/** Shift base lightness by discrete shade tick (`SHADE_TICK_NEUTRAL` = neutral). */
function lightnessAfterShadeTone(baseLightness: number, shadeTick: number): number {
	const tick = Math.round(Math.max(SHADE_TICK_MIN, Math.min(SHADE_TICK_MAX, shadeTick)));
	const halfSpan = SHADE_TICK_NEUTRAL - SHADE_TICK_MIN;
	const t = ((tick - SHADE_TICK_NEUTRAL) / halfSpan) * SHADE_TONE_ATTENUATION;
	const delta = t * SHADE_LIGHTNESS_DELTA_MAX;
	const l = baseLightness + delta;
	return Math.min(Math.max(l, SOFT_L_MIN), SOFT_L_MAX);
}

/**
 * Parses any CSS color string to sRGB using a canvas (browser only).
 * Falls back to mid-gray when not in browser or parse fails.
 */
export function cssColorToRgb(css: string): { r: number; g: number; b: number } {
	if (typeof document === 'undefined') {
		return { r: 128, g: 128, b: 128 };
	}
	try {
		const canvas = document.createElement('canvas');
		canvas.width = 1;
		canvas.height = 1;
		const ctx = canvas.getContext('2d');
		if (!ctx) {
			return { r: 128, g: 128, b: 128 };
		}
		ctx.fillStyle = css;
		ctx.fillRect(0, 0, 1, 1);
		const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
		return { r, g, b };
	} catch {
		return { r: 128, g: 128, b: 128 };
	}
}

export function rgbToHsl(r: number, g: number, b: number): { h: number; s: number; l: number } {
	r /= 255;
	g /= 255;
	b /= 255;
	const max = Math.max(r, g, b);
	const min = Math.min(r, g, b);
	let h = 0;
	let s = 0;
	const l = (max + min) / 2;

	if (max !== min) {
		const d = max - min;
		s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
		switch (max) {
			case r:
				h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
				break;
			case g:
				h = ((b - r) / d + 2) / 6;
				break;
			default:
				h = ((r - g) / d + 4) / 6;
		}
	}

	return { h: h * 360, s, l };
}

export function hslToRgb(h: number, s: number, l: number): { r: number; g: number; b: number } {
	let hh = ((h % 360) + 360) % 360;
	hh /= 360;

	if (s === 0) {
		const v = Math.round(l * 255);
		return { r: v, g: v, b: v };
	}

	const hue2rgb = (p: number, q: number, t: number) => {
		let tt = t;
		if (tt < 0) tt += 1;
		if (tt > 1) tt -= 1;
		if (tt < 1 / 6) return p + (q - p) * 6 * tt;
		if (tt < 1 / 2) return q;
		if (tt < 2 / 3) return p + (q - p) * (2 / 3 - tt) * 6;
		return p;
	};

	const q = l < 0.5 ? l * (1 + s) : l + s - l * s;
	const p = 2 * l - q;
	const r = hue2rgb(p, q, hh + 1 / 3);
	const g = hue2rgb(p, q, hh);
	const b = hue2rgb(p, q, hh - 1 / 3);

	return {
		r: Math.round(r * 255),
		g: Math.round(g * 255),
		b: Math.round(b * 255)
	};
}

function rgbToCssRgb(r: number, g: number, b: number): string {
	const clamp = (n: number) => Math.max(0, Math.min(255, Math.round(n)));
	return `rgb(${clamp(r)} ${clamp(g)} ${clamp(b)})`;
}

/**
 * Computes one surface/background color from a base CSS color, hue (degrees), tint (0–100), and shade
 * (integer ticks `SHADE_TICK_MIN`–`SHADE_TICK_MAX`, neutral `SHADE_TICK_NEUTRAL`).
 * - Tint 0: preserves base hue/saturation (no chromatic tint); lightness uses the shade-toned base.
 * - Tint > 0: hue/sat from tint; lightness follows the toned base, clamped so chroma shows on whites.
 * - Shade neutral tick: no tone shift. Otherwise adjusts underlying HSL lightness (darker/lighter palette).
 */
export function applyHueTintShadeToSurface(
	baseCss: string,
	hueDeg: number,
	tint0to100: number,
	shadeTick: number
): string {
	const { r: br, g: bg, b: bb } = cssColorToRgb(baseCss);
	const { h: bh, s: bs, l: bl } = rgbToHsl(br, bg, bb);

	const blTone = lightnessAfterShadeTone(bl, shadeTick);

	const tint = Math.max(0, Math.min(100, tint0to100));
	let h = bh;
	let s = bs;
	let lForHsl = blTone;

	if (tint > 0) {
		h = ((hueDeg % 360) + 360) % 360;
		s = (tint / 100) * TINT_MAX_SATURATION;
		lForHsl = clampLightnessForHslChroma(blTone);
	}

	const { r, g, b } = hslToRgb(h, s, lForHsl);
	return rgbToCssRgb(r, g, b);
}

export type TintedSurfaceSnapshot = {
	light: {
		backgroundColor: string;
		surface1Color: string;
		surface2Color: string;
		surface3Color: string;
	};
	dark: {
		darkBackgroundColor: string;
		darkSurface1Color: string;
		darkSurface2Color: string;
		darkSurface3Color: string;
	};
};

/** Hue / tint / shade for one color scheme’s tinted surfaces (light vs dark tracked separately). */
export type TintedSliderSet = {
	hueDeg: number;
	tint0to100: number;
	shadeTick: number;
};

/**
 * Builds theme patch objects for light + dark surface keys from tinted snapshots and per-scheme sliders.
 */
export function computeTintedThemePatch(
	snapshot: TintedSurfaceSnapshot,
	lightSliders: TintedSliderSet,
	darkSliders: TintedSliderSet
): Record<string, string> {
	const { light, dark } = snapshot;
	const L = lightSliders;
	const D = darkSliders;
	return {
		backgroundColor: applyHueTintShadeToSurface(
			light.backgroundColor,
			L.hueDeg,
			L.tint0to100,
			L.shadeTick
		),
		surface1Color: applyHueTintShadeToSurface(
			light.surface1Color,
			L.hueDeg,
			L.tint0to100,
			L.shadeTick
		),
		surface2Color: applyHueTintShadeToSurface(
			light.surface2Color,
			L.hueDeg,
			L.tint0to100,
			L.shadeTick
		),
		surface3Color: applyHueTintShadeToSurface(
			light.surface3Color,
			L.hueDeg,
			L.tint0to100,
			L.shadeTick
		),
		darkBackgroundColor: applyHueTintShadeToSurface(
			dark.darkBackgroundColor,
			D.hueDeg,
			D.tint0to100,
			D.shadeTick
		),
		darkSurface1Color: applyHueTintShadeToSurface(
			dark.darkSurface1Color,
			D.hueDeg,
			D.tint0to100,
			D.shadeTick
		),
		darkSurface2Color: applyHueTintShadeToSurface(
			dark.darkSurface2Color,
			D.hueDeg,
			D.tint0to100,
			D.shadeTick
		),
		darkSurface3Color: applyHueTintShadeToSurface(
			dark.darkSurface3Color,
			D.hueDeg,
			D.tint0to100,
			D.shadeTick
		)
	};
}
