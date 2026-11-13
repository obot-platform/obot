import type { AuditLogDetails } from '$lib/services';

// Obot Sentry leaves the tool input and output out of a local-agent hook's raw event, because the
// same values are submitted as the request and response bodies. In their place it writes a marker
// naming the field that holds the value, for example
// "tool_response": {"obot_sentry_omitted": "details.response.body"}. A marker can also say the
// original was a JSON string holding that value as compact JSON ("encoding": "json_string").
const omittedKey = 'obot_sentry_omitted';
const encodingKey = 'encoding';
const jsonStringEncoding = 'json_string';

type OmittedMarker = {
	field: 'details.request.body' | 'details.response.body';
	jsonString: boolean;
};

function parseMarker(value: unknown): OmittedMarker | undefined {
	if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
	const entries = Object.entries(value);
	const marker = Object.fromEntries(entries);
	const field = marker[omittedKey];
	if (field !== 'details.request.body' && field !== 'details.response.body') return undefined;
	if (entries.length === 1) return { field, jsonString: false };
	if (entries.length === 2 && marker[encodingKey] === jsonStringEncoding) {
		return { field, jsonString: true };
	}
	return undefined;
}

// restoreRawEvent returns the raw event with each Obot Sentry marker replaced by the value it
// stands for, so the raw event reads as the hook payload the agent sent. Key order is kept. A
// marker whose value is not in the log is left as it is.
export function restoreRawEvent(rawEvent: unknown, details: AuditLogDetails): unknown {
	if (!rawEvent || typeof rawEvent !== 'object' || Array.isArray(rawEvent)) return rawEvent;

	let restored = false;
	const entries = Object.entries(rawEvent).map(([key, value]) => {
		const marker = parseMarker(value);
		if (!marker) return [key, value];
		const payload = marker.field === 'details.request.body' ? details.request : details.response;
		if (!payload || payload.body === undefined) return [key, value];
		restored = true;
		return [key, marker.jsonString ? JSON.stringify(payload.body) : payload.body];
	});

	return restored ? Object.fromEntries(entries) : rawEvent;
}
