import { redirect } from '@sveltejs/kit';
import { session } from '#lib/stores/session.svelte.js';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ url }) => {
	await session.ensure();
	if (!session.me) {
		redirect(307, `/login?next=${encodeURIComponent(url.pathname + url.search)}`);
	}
};
