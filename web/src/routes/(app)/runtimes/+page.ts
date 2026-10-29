import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageLoad } from './$types';

// Runtimes has one page per runtime; the first one in the menu opens.
export const load: PageLoad = () =>
	redirect(307, resolve('/(app)/runtimes/[name]', { name: 'node' }));
