import { requirePermission } from '#lib/guard.js';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = ({ parent }) => requirePermission(parent, 'user.manage');
