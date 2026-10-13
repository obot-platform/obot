// Errors lose their non-enumerable message when passed directly to JSON.stringify.
// Select protocol/validation fields and causes rather than serializing request or transport objects.
export function testerErrorDetails(error: unknown): unknown {
	const seen = new WeakSet<object>();
	const serialize = (value: unknown): unknown => {
		if (!value || typeof value !== 'object') return value;
		if (seen.has(value)) return '[Circular]';
		seen.add(value);
		if (value instanceof Error) {
			const details: Record<string, unknown> = { name: value.name, message: value.message };
			for (const key of ['code', 'data', 'issues', 'cause']) {
				if (key in value)
					details[key] = serialize((value as unknown as Record<string, unknown>)[key]);
			}
			return details;
		}
		if (Array.isArray(value)) return value.map(serialize);
		return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, serialize(child)]));
	};
	return serialize(error);
}
