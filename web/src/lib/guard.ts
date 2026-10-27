import { error } from '@sveltejs/kit';
import type { Action } from '#lib/api/types.gen.js';
import { session } from '#lib/stores/session.svelte.js';

/**
 * Page-load guard. SvelteKit runs page and layout loads in parallel, so the
 * (app) layout may still be fetching /me; `parent()` waits for it first.
 */
export async function requirePermission(
	parent: () => Promise<unknown>,
	action: Action,
	message = 'You do not have access to this page'
) {
	await parent();
	if (!session.can(action)) error(403, message);
}
