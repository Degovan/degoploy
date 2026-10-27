import { error } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => {
	const did = Number(params.did);
	if (!Number.isInteger(did) || did <= 0) error(404, 'Deployment not found');
	return { did };
};
