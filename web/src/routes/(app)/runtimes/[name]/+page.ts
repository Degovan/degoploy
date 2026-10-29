import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => ({ name: params.name });
