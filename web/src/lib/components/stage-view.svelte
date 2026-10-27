<script lang="ts">
	import { resolve } from '$app/paths';
	import { isActive } from '#lib/api/deployments.js';
	import type { Deployment, Project } from '#lib/api/types.gen.js';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import StatusIcon from '#lib/components/status-icon.svelte';
	import { averageStages, deploymentStages, scriptLabel, type Stage } from '#lib/stages.js';
	import { formatStage } from '#lib/time.js';
	import { cn } from '#lib/utils.js';

	// Jenkins-style stage view: one row per deployment, one column per stage.
	let { project: p, deployments }: { project: Project; deployments: Deployment[] } = $props();

	let now = $state(Date.now());
	$effect(() => {
		if (!deployments.some(isActive)) return;
		const t = setInterval(() => (now = Date.now()), 500);
		return () => clearInterval(t);
	});

	const avg = $derived(averageStages(deployments, p));
	const columns = $derived([
		{ key: 'trigger', label: 'Trigger' },
		{ key: 'update', label: 'Update repository' },
		{ key: 'script', label: scriptLabel(p) }
	] as const);

	const dateFmt = new Intl.DateTimeFormat(undefined, {
		month: 'short',
		day: 'numeric',
		hour: '2-digit',
		minute: '2-digit',
		hour12: false
	});

	function cellTone(s: Stage) {
		switch (s.state) {
			case 'success':
				return 'bg-emerald-500/[0.07]';
			case 'failed':
				return 'bg-red-500/15 text-red-600 dark:text-red-400';
			case 'running':
				return 'bg-amber-400/10 text-amber-700 dark:text-amber-300';
			default:
				return 'text-muted-foreground';
		}
	}

	function cellNote(s: Stage, d: Deployment) {
		if (s.state === 'failed') {
			if (d.status === 'timeout') return 'timed out';
			return d.exit_code !== null && s.key === 'script' ? `failed · exit ${d.exit_code}` : 'failed';
		}
		if (s.state === 'canceled') return d.status === 'skipped' ? 'skipped' : 'canceled';
		if (s.state === 'skipped') return d.started_at ? 'skipped' : '';
		if (s.state === 'running') return 'running';
		return '';
	}
</script>

<section class="flex flex-col gap-4" aria-labelledby="stage-view">
	<SectionHeading
		title="Stage View"
		description="Each deployment broken down by stage, newest first."
		id="stage-view"
	/>

	<div class="overflow-x-auto rounded-xl border bg-card">
		<table class="w-full min-w-[40rem] border-collapse text-sm" data-testid="stage-view">
			<thead>
				<tr class="border-b">
					<th class="w-[38%] px-5 py-3 text-left font-normal">
						<span class="block text-xs text-muted-foreground">Average stage times</span>
						<span class="block font-mono text-xs text-muted-foreground">
							full run ≈ {avg.run === null ? '—' : formatStage(avg.run)}
						</span>
					</th>
					{#each columns as c (c.key)}
						<th class="border-l px-4 py-3 text-center font-normal">
							<span class="block truncate text-sm font-medium" class:font-mono={c.key === 'script'}
								>{c.label}</span
							>
							<span class="block font-mono text-xs text-muted-foreground">
								{avg.stages[c.key] === null ? '—' : formatStage(avg.stages[c.key]!)}
							</span>
						</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each deployments as d (d.id)}
					{@const stages = deploymentStages(d, p, now)}
					<tr class="border-b last:border-b-0">
						<td class="px-5 py-3">
							<a
								href={resolve('/(app)/projects/[id]/deployments/[did]', {
									id: String(p.id),
									did: String(d.id)
								})}
								class="flex min-w-0 items-center gap-3 hover:[&_.msg]:underline"
							>
								<StatusIcon status={d.status} />
								<span class="flex min-w-0 flex-col">
									<span class="msg truncate">
										<span class="font-medium">#{d.id}</span>
										{d.commit_message.split('\n')[0]}
									</span>
									<span class="text-xs text-muted-foreground">
										{dateFmt.format(new Date(d.queued_at))} · {d.trigger}
									</span>
								</span>
							</a>
						</td>
						{#each stages as s (s.key)}
							{@const note = cellNote(s, d)}
							<td
								class={cn('border-l px-4 py-3 text-center font-mono text-sm', cellTone(s))}
								data-state={s.state}
							>
								{s.ms === null ? '' : formatStage(s.ms)}
								{#if note}<span class="block text-xs font-normal opacity-80">{note}</span>{/if}
							</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</section>
