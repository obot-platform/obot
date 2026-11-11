// hasCookie reports whether the server set the cookie name to true.
export function hasCookie(name: string): boolean {
	if (typeof document === 'undefined') {
		return false;
	}
	return document.cookie.split(';').some((cookie) => cookie.trim() === `${name}=true`);
}
