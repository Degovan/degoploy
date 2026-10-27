<script lang="ts">
	import { resolve } from '$app/paths';
	import { shortRef } from '#lib/api/deployments.js';
	import type { Project } from '#lib/api/types.gen.js';
	import HealthIcon, { healthPercent, healthSummary } from '#lib/components/health-icon.svelte';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import ProviderIcon from '#lib/components/provider-icon.svelte';
	import StatusDot from '#lib/components/status-dot.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { parseRepo, providerLabel, providerLinks } from '#lib/repo.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import PauseIcon from '@lucide/svelte/icons/pause';

	let { project: p, now }: { project: Project; now: number } = $props();

	const d = $derived(p.last_deployment);
	const links = $derived(providerLinks(p.provider, p.repo_url));
	const id = $derived(String(p.id));
</script>

<article
	class="group relative flex flex-col gap-5 rounded-xl border bg-card p-6 transition-colors focus-within:border-foreground/30 hover:border-foreground/25"
	data-testid="project-card"
>
	<div class={cn('flex items-start gap-3', !p.enabled && 'opacity-50')}>
		<ProjectAvatar seed={p.slug} muted={!p.enabled} class="mt-0.5 size-9" />
		<div class="flex min-w-0 flex-1 flex-col">
			<h3 class="truncate font-medium">
				<!-- The link covers the whole card; the menu sits above it. -->
				<a
					href={resolve('/(app)/projects/[id]', { id })}
					class="outline-none after:absolute after:inset-0 after:rounded-xl"
				>
					{p.name}
				</a>
			</h3>
			<span class="truncate font-mono text-sm text-muted-foreground" title={p.work_dir}
				>{p.work_dir}</span
			>
		</div>
		{#if p.health}
			<span
				class="flex shrink-0 items-center gap-1.5 pt-1 font-mono text-xs text-muted-foreground"
				title={healthSummary(p.health)}
			>
				<HealthIcon health={p.health} />
				{healthPercent(p.health)}%
			</span>
		{/if}
		<div class="relative z-10 -mt-1 -mr-2 shrink-0">
			<DropdownMenu.Root>
				<DropdownMenu.Trigger>
					{#snippet child({ props })}
						<Button variant="ghost" size="icon-sm" aria-label={`Actions for ${p.name}`} {...props}>
							<EllipsisIcon />
						</Button>
					{/snippet}
				</DropdownMenu.Trigger>
				<DropdownMenu.Content align="end" class="w-48">
					<DropdownMenu.Item>
						{#snippet child({ props })}
							<a href={resolve('/(app)/projects/[id]', { id })} {...props}>Overview</a>
						{/snippet}
					</DropdownMenu.Item>
					<DropdownMenu.Item>
						{#snippet child({ props })}
							<a href={resolve('/(app)/projects/[id]/triggers', { id })} {...props}>Triggers</a>
						{/snippet}
					</DropdownMenu.Item>
					{#if session.can('project.edit')}
						<DropdownMenu.Item>
							{#snippet child({ props })}
								<a href={resolve('/(app)/projects/[id]/settings', { id })} {...props}>Settings</a>
							{/snippet}
						</DropdownMenu.Item>
					{/if}
					{#if links}
						<DropdownMenu.Separator />
						<DropdownMenu.Item>
							{#snippet child({ props })}
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external link -->
								<a href={links.repo} target="_blank" rel="noreferrer" {...props}>
									Open on {providerLabel[p.provider]}
									<ExternalLinkIcon class="ml-auto" />
								</a>
							{/snippet}
						</DropdownMenu.Item>
					{/if}
				</DropdownMenu.Content>
			</DropdownMenu.Root>
		</div>
	</div>

	<div class={cn('flex flex-wrap gap-2', !p.enabled && 'opacity-70')}>
		<span
			class="inline-flex h-7 max-w-full items-center gap-1.5 rounded-full bg-muted px-3 text-sm"
		>
			<ProviderIcon provider={p.provider} class="size-3.5 shrink-0 text-muted-foreground" />
			<span class="truncate">{parseRepo(p.repo_url)?.path ?? p.repo_url}</span>
		</span>
		{#if !p.enabled}
			<span
				class="inline-flex h-7 items-center gap-1.5 rounded-full border px-3 text-sm text-muted-foreground"
			>
				<PauseIcon class="size-3.5" />
				Disabled
			</span>
		{/if}
	</div>

	<div class={cn('mt-auto flex min-w-0 flex-col gap-1.5', !p.enabled && 'opacity-50')}>
		{#if d}
			<p class="truncate font-medium">{d.commit_message.split('\n')[0] || 'No commit message'}</p>
			<div class="flex min-w-0 items-center gap-1.5 text-sm text-muted-foreground">
				<StatusDot status={d.status} class="shrink-0 text-foreground/80" />
				<span class="shrink-0">#{d.id}</span>
				<span class="shrink-0" title={formatDateTime(d.queued_at)}
					>· {shortAgo(d.queued_at, now)} on</span
				>
				<GitBranchIcon class="size-3.5 shrink-0" />
				<span class="truncate font-mono">{shortRef(d.ref)}</span>
			</div>
		{:else}
			<p class="font-medium text-muted-foreground">Never deployed</p>
			<p class="text-sm text-muted-foreground">Waiting for the first push or a manual deploy.</p>
		{/if}
	</div>
</article>
