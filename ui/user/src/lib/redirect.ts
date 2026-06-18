export function safeRedirectPath(path: string | null | undefined, appBasePath = ''): string | null {
	if (!path || !path.startsWith('/') || path.startsWith('//') || path.includes('\\')) {
		return null;
	}

	if (!appBasePath || path === appBasePath || path.startsWith(`${appBasePath}/`)) {
		return path;
	}

	return `${appBasePath}${path}`;
}
