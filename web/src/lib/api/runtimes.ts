import { api } from './client.js';
import type {
	RuntimeProgress,
	RuntimeRelease,
	RuntimeStatus,
	RuntimeUpdateResult
} from './types.gen.js';

export const runtimeKeys = {
	all: ['runtimes'] as const,
	releases: (name: string) => ['runtimes', name, 'releases'] as const
};

const base = (name: string) => `/runtimes/${encodeURIComponent(name)}`;

export const runtimesApi = {
	list: () => api.get<RuntimeStatus[]>('/runtimes'),
	releases: (name: string) => api.get<RuntimeRelease[]>(`${base(name)}/releases`),
	install: (name: string, spec: string) =>
		api.post<RuntimeStatus>(`${base(name)}/install`, { spec }),
	remove: (name: string, version: string) =>
		api.delete<RuntimeStatus>(`${base(name)}/versions/${encodeURIComponent(version)}`),
	setDefault: (name: string, spec: string) =>
		api.put<RuntimeStatus>(`${base(name)}/default`, { spec }),
	setEnabled: (name: string, enabled: boolean) =>
		api.put<RuntimeStatus>(`${base(name)}/enabled`, { enabled }),
	update: (name: string) => api.post<RuntimeUpdateResult>(`${base(name)}/update`),
	cancel: (name: string, version: string) =>
		api.post<void>(`${base(name)}/installing/${encodeURIComponent(version)}/cancel`)
};

/** Display names, for when the runtime list is not loaded. */
export const runtimeTitles: Record<string, string> = { node: 'Node.js', php: 'PHP' };

/** A project's runtime setting in a word: "Off", "Auto" or the version spec. */
export const specLabel = (spec: string) =>
	spec === 'off' ? 'Off' : spec === 'auto' ? 'Auto' : spec;

/** A version's release line: "22" for Node.js, "8.3" for PHP. */
export function lineOf(runtime: string, version: string): string {
	const parts = version.replace(/^v/, '').split('.');
	return runtime === 'php' ? parts.slice(0, 2).join('.') : (parts[0] ?? version);
}

/** Examples for inputs and hints, per runtime. */
export const runtimeExamples: Record<string, { line: string; exact: string; files: string[] }> = {
	node: { line: '22', exact: '20.11.1', files: ['.nvmrc', '.node-version', 'package.json'] },
	php: { line: '8.3', exact: '8.3.14', files: ['.php-version', 'composer.json'] }
};

export function formatSize(bytes: number): string {
	if (bytes <= 0) return '–';
	if (bytes < 1 << 20) return `${Math.round(bytes / 1024)} KB`;
	if (bytes < 1 << 30) return `${Math.round(bytes / (1 << 20))} MB`;
	return `${(bytes / (1 << 30)).toFixed(1)} GB`;
}

/** Whole percent downloaded, or null when the size is unknown. */
export const progressPercent = (p: RuntimeProgress) =>
	p.total > 0 ? Math.min(100, Math.floor((p.done / p.total) * 100)) : null;

/** "45% · 24.3 of 54.2 MB", or "24.3 MB" when the size is unknown. */
export function formatProgress(p: RuntimeProgress): string {
	const mb = (n: number) => (n / (1 << 20)).toFixed(1);
	const pct = progressPercent(p);
	return pct === null ? `${mb(p.done)} MB` : `${pct}% · ${mb(p.done)} of ${mb(p.total)} MB`;
}
