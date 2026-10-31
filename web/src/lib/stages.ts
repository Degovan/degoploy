import type { Deployment, Project } from '#lib/api/types.gen.js';

export type StageKey = 'trigger' | 'update' | 'script';
export type StageState = 'pending' | 'running' | 'success' | 'failed' | 'canceled' | 'skipped';

export interface Stage {
	key: StageKey;
	label: string;
	state: StageState;
	/** Elapsed time; null when unknown or the stage did not run. */
	ms: number | null;
}

const t = (iso: string | null) => (iso ? new Date(iso).getTime() : null);

/** File name of the deploy script, used as the script stage's label. */
export function scriptLabel(p: Pick<Project, 'script_source' | 'script_path'>): string {
	if (p.script_source === 'managed') return 'deploy.sh';
	return p.script_path.split('/').pop() || 'deploy script';
}

/**
 * Splits a deployment into trigger (waiting in the queue), repository update and
 * script stages from its timestamps. Deployments recorded before degoploy kept
 * script_started_at have no update/script split: their update time is unknown.
 */
export function deploymentStages(
	d: Deployment,
	p: Pick<Project, 'auto_pull' | 'script_source' | 'script_path'>,
	now = Date.now()
): Stage[] {
	const queued = t(d.queued_at)!;
	const started = t(d.started_at);
	const scriptStarted = t(d.script_started_at);
	const finished = t(d.finished_at);
	const done = finished !== null;
	// What this run did; the project's current setting for runs that have not started
	// (or were recorded before degoploy kept it).
	const pulls = d.auto_pull ?? p.auto_pull;
	const ended: StageState =
		d.status === 'success'
			? 'success'
			: d.status === 'canceled' || d.status === 'skipped'
				? 'canceled'
				: 'failed';

	const trigger: Stage = { key: 'trigger', label: 'Trigger', state: 'pending', ms: null };
	const update: Stage = { key: 'update', label: 'Update repo', state: 'pending', ms: null };
	// A rollback runs no script: its last stage switches current to the release.
	const script: Stage = {
		key: 'script',
		label: d.trigger === 'rollback' ? 'Switch release' : scriptLabel(p),
		state: 'pending',
		ms: null
	};

	if (started === null) {
		// Never left the queue: still waiting, or skipped/canceled while queued.
		trigger.state = done ? 'canceled' : 'running';
		trigger.ms = (finished ?? now) - queued;
		if (done) update.state = script.state = 'skipped';
		if (!pulls) update.state = 'skipped';
		return [trigger, update, script];
	}
	trigger.state = 'success';
	trigger.ms = started - queued;

	if (scriptStarted !== null) {
		if (pulls) {
			update.state = 'success';
			update.ms = scriptStarted - started;
		} else {
			update.state = 'skipped';
		}
		script.state = done ? ended : 'running';
		script.ms = (finished ?? now) - scriptStarted;
	} else if (!done) {
		// Running, script not started yet: updating the repository (or resolving the script).
		const stage = pulls ? update : script;
		stage.state = 'running';
		stage.ms = now - started;
		if (!pulls) update.state = 'skipped';
	} else if (d.status === 'success') {
		// Recorded before stage times existed.
		update.state = pulls ? 'success' : 'skipped';
		script.state = 'success';
		script.ms = finished - started;
	} else {
		// Ended before the script started: the update (or script lookup) failed or was canceled.
		const stage = pulls ? update : script;
		stage.state = ended;
		stage.ms = finished - started;
		if (pulls) script.state = 'skipped';
		else update.state = 'skipped';
	}
	return [trigger, update, script];
}

/** Total run time (start to finish, or so far), excluding time in the queue. */
export function runMs(d: Deployment, now = Date.now()): number | null {
	const started = t(d.started_at);
	if (started === null) return null;
	return (t(d.finished_at) ?? now) - started;
}

const mean = (xs: number[]) => (xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null);

/** Average stage and run times over successful deployments. */
export function averageStages(
	deployments: Deployment[],
	p: Pick<Project, 'auto_pull' | 'script_source' | 'script_path'>
): { stages: Record<StageKey, number | null>; run: number | null } {
	const ok = deployments.filter((d) => d.status === 'success');
	const per: Record<StageKey, number[]> = { trigger: [], update: [], script: [] };
	for (const d of ok) {
		for (const s of deploymentStages(d, p)) {
			if (s.state === 'success' && s.ms !== null) per[s.key].push(s.ms);
		}
	}
	return {
		stages: { trigger: mean(per.trigger), update: mean(per.update), script: mean(per.script) },
		run: mean(ok.map((d) => runMs(d)).filter((x): x is number => x !== null))
	};
}
