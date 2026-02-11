export function parseColorToHsl(css: string): { h: number; s: number; l: number } | null {
	const s = css.trim();
	const hexMatch = s.match(/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$/);
	if (hexMatch) {
		let hex = hexMatch[1];
		if (hex.length === 3) hex = hex[0] + hex[0] + hex[1] + hex[1] + hex[2] + hex[2];
		const r = parseInt(hex.slice(0, 2), 16) / 255;
		const g = parseInt(hex.slice(2, 4), 16) / 255;
		const b = parseInt(hex.slice(4, 6), 16) / 255;
		const max = Math.max(r, g, b);
		const min = Math.min(r, g, b);
		let h = 0;
		let s_ = 0;
		const l = (max + min) / 2;
		if (max !== min) {
			const d = max - min;
			s_ = l > 0.5 ? d / (2 - max - min) : d / (max + min);
			switch (max) {
				case r:
					h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
					break;
				case g:
					h = ((b - r) / d + 2) / 6;
					break;
				case b:
					h = ((r - g) / d + 4) / 6;
					break;
			}
		}
		return { h: h * 360, s: s_ * 100, l: l * 100 };
	}
	const rgbMatch = s.match(/^rgba?\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)/);
	if (rgbMatch) {
		const r = parseInt(rgbMatch[1], 10) / 255;
		const g = parseInt(rgbMatch[2], 10) / 255;
		const b = parseInt(rgbMatch[3], 10) / 255;
		const max = Math.max(r, g, b);
		const min = Math.min(r, g, b);
		let h = 0;
		let s_ = 0;
		const l = (max + min) / 2;
		if (max !== min) {
			const d = max - min;
			s_ = l > 0.5 ? d / (2 - max - min) : d / (max + min);
			switch (max) {
				case r:
					h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
					break;
				case g:
					h = ((b - r) / d + 2) / 6;
					break;
				case b:
					h = ((r - g) / d + 4) / 6;
					break;
			}
		}
		return { h: h * 360, s: s_ * 100, l: l * 100 };
	}
	const hslMatch = s.match(/^hsla?\s*\(\s*([\d.]+)\s*[, ]\s*([\d.]+)%?\s*[, ]\s*([\d.]+)%?\s*\)/);
	if (hslMatch) {
		return {
			h: parseFloat(hslMatch[1]) % 360,
			s: Math.min(100, Math.max(0, parseFloat(hslMatch[2]))),
			l: Math.min(100, Math.max(0, parseFloat(hslMatch[3])))
		};
	}
	return null;
}

export function hslToHex(h: number, s: number, l: number): string {
	s /= 100;
	l /= 100;
	const a = s * Math.min(l, 1 - l);
	const f = (n: number) => {
		const k = (n + h / 30) % 12;
		const v = l - a * Math.max(Math.min(k - 3, 9 - k, 1), -1);
		return Math.round(v * 255)
			.toString(16)
			.padStart(2, '0');
	};
	return `#${f(0)}${f(8)}${f(4)}`;
}

const PALETTE_SIZE = 9;

/** Build a contrasting palette from primary: primary first, then evenly spaced hues. */
export function buildPaletteFromPrimary(primaryHsl: { h: number; s: number; l: number }): string[] {
	const { h, s, l } = primaryHsl;
	const sat = Math.max(50, Math.min(80, s));
	const light = Math.max(42, Math.min(58, l));
	const out: string[] = [];
	for (let i = 0; i < PALETTE_SIZE; i++) {
		const hue = (h + (360 / PALETTE_SIZE) * i) % 360;
		out.push(hslToHex(hue, sat, light));
	}
	return out;
}

export function lightenHex(hex: string, amount: number): string {
	const h = hex.replace(/^#/, '');
	const r = parseInt(h.slice(0, 2), 16);
	const g = parseInt(h.slice(2, 4), 16);
	const b = parseInt(h.slice(4, 6), 16);
	const mix = (c: number) => Math.round(c + (255 - c) * amount);
	return `#${[r, g, b].map((c) => mix(c).toString(16).padStart(2, '0')).join('')}`;
}
