import { redirect } from '@sveltejs/kit';
import { session } from '#lib/stores/session.svelte.js';
import { safeNext } from '#lib/redirect.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ url }) => {
	await session.ensure();
	const next = safeNext(url.searchParams.get('next'));
	if (session.me) redirect(307, next);
	// Set by the server when an SSO sign-in fails (see internal/api/oidc.go).
	return { next, ssoError: url.searchParams.get('error') ?? '' };
};
