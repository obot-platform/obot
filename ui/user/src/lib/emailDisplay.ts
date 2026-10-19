// Only use for rendering. Authentication, edits and API requests keep user.email.
export function formatUserEmail(user: { email?: string; emailDisplayDomain?: string }): string {
	const rawEmail = user.email ?? '';
	const domain = (user.emailDisplayDomain ?? '').trim().replace(/^@/, '').toLowerCase();
	if (!domain) return rawEmail;
	const email = rawEmail.toLowerCase();
	return email && !email.includes('@') ? `${email}@${domain}` : email;
}
