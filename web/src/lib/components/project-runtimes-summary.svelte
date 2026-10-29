<script lang="ts">
	import { resolve } from '$app/paths';
	import { runtimeTitles, specLabel } from '#lib/api/runtimes.js';
	import type { Project } from '#lib/api/types.gen.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import LayersIcon from '@lucide/svelte/icons/layers';

	// The project's runtimes beside its deploy script, linking to where they are set.
	let { project }: { project: Project } = $props();

	const detail = (name: string, spec: string, used: string) =>
		spec === 'off'
			? `Not managed: the server's own ${name}, if any`
			: used
				? `Last deployment: ${used}`
				: spec === 'auto'
					? 'When the repository uses it'
					: 'Always on';
</script>

<section class="flex flex-col gap-4 rounded-xl border bg-card p-6" aria-label="Runtimes">
	<div class="flex flex-col gap-1">
		<h2 class="flex items-center gap-2 text-sm font-semibold">
			<LayersIcon class="size-4 text-muted-foreground" />
			Runtimes
		</h2>
		<p class="text-xs text-muted-foreground/80">
			On the script's PATH, with npm, corepack and Composer.
		</p>
	</div>
	<dl class="flex flex-col gap-3">
		{#each project.runtimes as r (r.name)}
			<div class="flex items-start justify-between gap-3">
				<div class="flex min-w-0 flex-col gap-0.5">
					<dt class="text-[13px] font-medium">{runtimeTitles[r.name] ?? r.name}</dt>
					<dd class="text-xs text-muted-foreground/80">{detail(r.name, r.spec, r.used)}</dd>
				</div>
				<span
					class={cn(
						'inline-flex h-[22px] shrink-0 items-center rounded-[5px] border px-[7px] text-xs',
						r.spec === 'off'
							? 'border-input text-muted-foreground'
							: 'border-muted-foreground/40 bg-muted font-medium',
						r.spec !== 'off' && r.spec !== 'auto' && 'font-mono'
					)}>{specLabel(r.spec)}</span
				>
			</div>
		{/each}
	</dl>
	{#if session.can('project.edit')}
		<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved, plus a section hash -->
		<a
			href={`${resolve('/(app)/projects/[id]/settings', { id: String(project.id) })}#runtimes`}
			class="-mx-2 -mb-2 flex items-center justify-between rounded-md px-2 py-1.5 text-[13px] text-muted-foreground hover:bg-accent hover:text-foreground"
		>
			Change runtimes
			<ArrowRightIcon class="size-3.5" />
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	{/if}
</section>
