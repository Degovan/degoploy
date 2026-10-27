const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
const full = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' });

const units: [Intl.RelativeTimeFormatUnit, number][] = [
	['year', 365 * 24 * 3600],
	['month', 30 * 24 * 3600],
	['day', 24 * 3600],
	['hour', 3600],
	['minute', 60],
	['second', 1]
];

/** "3 minutes ago" style label for an ISO timestamp. */
export function timeAgo(iso: string, now = Date.now()): string {
	const secs = Math.round((new Date(iso).getTime() - now) / 1000);
	for (const [unit, size] of units) {
		if (Math.abs(secs) >= size || unit === 'second') {
			return rtf.format(Math.round(secs / size), unit);
		}
	}
	return '';
}

export function formatDateTime(iso: string): string {
	return full.format(new Date(iso));
}

/** Compact duration: "850ms", "42s", "3m 05s", "1h 02m". */
export function formatDuration(ms: number): string {
	if (ms < 1000) return `${Math.max(0, Math.round(ms))}ms`;
	const s = Math.floor(ms / 1000);
	if (s < 60) return `${s}s`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m}m ${String(s % 60).padStart(2, '0')}s`;
	return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, '0')}m`;
}

/** Time a deployment took (or has taken so far, if still running). */
export function deploymentDuration(
	d: { started_at: string | null; finished_at: string | null },
	now = Date.now()
): number | null {
	if (!d.started_at) return null;
	const end = d.finished_at ? new Date(d.finished_at).getTime() : now;
	return end - new Date(d.started_at).getTime();
}
