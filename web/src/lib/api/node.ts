import { api } from './client.js';
import type { NodeProgress, NodeRelease, NodeStatus, NodeUpdateResult } from './types.gen.js';

export const nodeKeys = {
	status: ['node'] as const,
	releases: ['node', 'releases'] as const
};

export const nodeApi = {
	status: () => api.get<NodeStatus>('/node'),
	releases: () => api.get<NodeRelease[]>('/node/releases'),
	install: (spec: string) => api.post<NodeStatus>('/node/install', { spec }),
	remove: (version: string) =>
		api.delete<NodeStatus>(`/node/versions/${encodeURIComponent(version)}`),
	setDefault: (spec: string) => api.put<NodeStatus>('/node/default', { spec }),
	update: () => api.post<NodeUpdateResult>('/node/update'),
	cancel: (version: string) =>
		api.post<void>(`/node/installing/${encodeURIComponent(version)}/cancel`)
};

/** "v22.12.0" → "22". */
export const majorOf = (version: string) => version.replace(/^v/, '').split('.')[0] ?? version;

export function formatSize(bytes: number): string {
	if (bytes <= 0) return '–';
	if (bytes < 1 << 20) return `${Math.round(bytes / 1024)} KB`;
	if (bytes < 1 << 30) return `${Math.round(bytes / (1 << 20))} MB`;
	return `${(bytes / (1 << 30)).toFixed(1)} GB`;
}

/** Whole percent downloaded, or null when the size is unknown. */
export const progressPercent = (p: NodeProgress) =>
	p.total > 0 ? Math.min(100, Math.floor((p.done / p.total) * 100)) : null;

/** "45% · 24.3 of 54.2 MB", or "24.3 MB" when the size is unknown. */
export function formatProgress(p: NodeProgress): string {
	const mb = (n: number) => (n / (1 << 20)).toFixed(1);
	const pct = progressPercent(p);
	return pct === null ? `${mb(p.done)} MB` : `${pct}% · ${mb(p.done)} of ${mb(p.total)} MB`;
}
