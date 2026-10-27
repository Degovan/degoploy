<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Deployment, Project } from '#lib/api/types.gen.js';
	import HealthIcon, { healthPercent, healthSummary } from '#lib/components/health-icon.svelte';
	import { averageStages } from '#lib/stages.js';
	import { formatStage } from '#lib/time.js';
	import SunIcon from '@lucide/svelte/icons/sun';

	let {
		project: p,
		deployments,
		ruleCount
	}: { project: Project; deployments: Deployment[]; ruleCount: number | null } = $props();

	const avg = $derived(averageStages(deployments, p).run);
	const created = $derived(
		new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(p.created_at))
	);
</script>

<div class="flex flex-col gap-5 rounded-xl border bg-card p-6" data-testid="project-summary">
	<h3 class="font-semibold">Project</h3>
	<div class="flex items-center gap-3 border-b pb-5">
		<span class="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-background">
			{#if p.health}
				<HealthIcon health={p.health} class="size-5" />
			{:else}
				<SunIcon class="size-5 text-muted-foreground" />
			{/if}
		</span>
		<div class="flex min-w-0 flex-col">
			{#if p.health}
				<span class="font-medium">{healthPercent(p.health)}% healthy</span>
				<span class="text-xs text-muted-foreground">{healthSummary(p.health)}</span>
			{:else}
				<span class="font-medium">No health yet</span>
				<span class="text-xs text-muted-foreground">Shown after the first finished deployment</span>
			{/if}
		</div>
	</div>
	<dl class="flex flex-col divide-y text-sm">
		{#snippet row(label: string, value: string, mono = false)}
			<div class="flex items-center justify-between gap-4 py-2.5 first:pt-0 last:pb-0">
				<dt class="text-muted-foreground">{label}</dt>
				<dd class={mono ? 'truncate font-mono text-xs' : 'truncate'}>{value}</dd>
			</div>
		{/snippet}
		{@render row('Avg. duration', avg === null ? '—' : formatStage(avg), true)}
		{@render row('Timeout', `${p.timeout_sec}s`, true)}
		{@render row('Before each run', p.auto_pull ? 'Update repository' : 'Nothing (script only)')}
		{@render row('Slug', p.slug, true)}
		<div class="flex items-center justify-between gap-4 py-2.5">
			<dt class="text-muted-foreground">Trigger rules</dt>
			<dd>
				<a
					href={resolve('/(app)/projects/[id]/triggers', { id: String(p.id) })}
					class="underline-offset-4 hover:underline"
				>
					{ruleCount === null ? '—' : `${ruleCount} rule${ruleCount === 1 ? '' : 's'}`}
				</a>
			</dd>
		</div>
		{@render row('Created', created)}
	</dl>
</div>
