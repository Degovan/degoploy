import { requirePermission } from '#lib/guard.js';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ parent }) =>
	requirePermission(parent, 'project.create', 'You do not have permission to create projects');
