import { session } from '#lib/stores/session.svelte.js';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	// Accepting signs in as the new account, replacing any current session.
	await session.ensure();
	return { token: params.token };
};
