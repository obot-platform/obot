// Accept existing comma-separated URLs as well as lossless JSON string arrays.
export function parseMultiValue(value: string | number | null | undefined): string[] {
	const text = String(value ?? '');
	try {
		const values: unknown = JSON.parse(text);
		if (Array.isArray(values) && values.every((item) => typeof item === 'string')) {
			return values.filter(Boolean);
		}
	} catch {
		// Existing URLs use comma-separated values.
	}
	return text
		.split(',')
		.map((item) => item.trim())
		.filter(Boolean);
}

export function serializeMultiValue(values: (string | number)[]): string {
	return values.length ? JSON.stringify(values.map(String)) : '';
}
