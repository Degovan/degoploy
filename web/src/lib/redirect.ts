/** Returns a same-origin path for post-login redirects; anything else falls back to /projects. */
export function safeNext(next: string | null): string {
	if (!next || !next.startsWith('/') || next.startsWith('//') || next.startsWith('/\\')) {
		return '/projects';
	}
	if (next === '/login' || next.startsWith('/login?')) return '/projects';
	return next;
}
