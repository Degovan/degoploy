import { error } from '@sveltejs/kit';
import { docBySlug } from '#lib/docs.js';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => {
	const slug = params.page ?? '';
	if (!docBySlug(slug)) error(404, 'This page is not in the guide.');
	return { slug };
};
