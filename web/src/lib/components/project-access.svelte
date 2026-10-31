<script lang="ts">
	import { createQuery } from '@tanstack/svelte-query';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { ProjectAccess, ProjectRole } from '#lib/api/types.gen.js';
	import ProjectAvatar from '#lib/components/project-avatar.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';

	// Every project, or a selection with a role on each. New picks start with defaultRole.
	let {
		selected = $bindable(false),
		projects = $bindable([]),
		defaultRole,
		error = ''
	}: {
		selected?: boolean;
		projects?: ProjectAccess[];
		defaultRole: ProjectRole;
		error?: string;
	} = $props();

	const list = createQuery(() => ({ queryKey: projectKeys.all, queryFn: projectsApi.list }));

	const roleOn = (id: number) => projects.find((p) => p.project_id === id)?.role;

	function toggle(id: number, on: boolean) {
		projects = on
			? [...projects, { project_id: id, role: defaultRole }]
			: projects.filter((p) => p.project_id !== id);
	}
	function setRole(id: number, role: ProjectRole) {
		projects = projects.map((p) => (p.project_id === id ? { ...p, role } : p));
	}

	const scopes = [
		{ value: false, label: 'All projects' },
		{ value: true, label: 'Selected projects' }
	];
	const projectRoles: { value: ProjectRole; label: string }[] = [
		{ value: 'deployer', label: 'Deployer' },
		{ value: 'viewer', label: 'Viewer' }
	];
	const segment =
		'flex h-full items-center rounded-sm px-3 text-[13px] font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-40';
</script>

<div class="flex flex-col gap-3">
	<div
		class="flex h-9 w-fit items-center gap-0.5 rounded-md border border-input p-[3px]"
		role="radiogroup"
		aria-label="Project access"
	>
		{#each scopes as s (s.label)}
			{@const active = selected === s.value}
			<button
				type="button"
				role="radio"
				aria-checked={active}
				class={cn(
					segment,
					active ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
				)}
				onclick={() => (selected = s.value)}
			>
				{s.label}
			</button>
		{/each}
	</div>

	{#if !selected}
		<p class="text-xs/normal text-muted-foreground/80">
			Their role applies to every project, including projects added later.
		</p>
	{:else}
		<p class="text-xs/normal text-muted-foreground/80">
			They see only these projects, each with its own role. Projects added later stay hidden until
			you pick them here.
		</p>
		{#if list.isPending}
			<Skeleton class="h-32 rounded-lg" />
		{:else if list.isError}
			<p class="flex items-center gap-2 text-[13px] text-destructive" role="alert">
				<CircleAlertIcon class="size-3.5" />
				{list.error.message}
			</p>
		{:else if list.data.length === 0}
			<p
				class="rounded-lg border border-dashed px-4 py-6 text-center text-[13px] text-muted-foreground"
			>
				No projects yet.
			</p>
		{:else}
			<ul
				class="max-h-64 [scrollbar-width:thin] overflow-y-auto overscroll-contain rounded-lg border bg-background"
				aria-label="Projects"
			>
				{#each list.data as p (p.id)}
					{@const role = roleOn(p.id)}
					<li class="flex items-center gap-3 border-b px-3.5 py-2.5 last:border-b-0">
						<label class="flex min-w-0 flex-1 cursor-pointer items-center gap-3">
							<input
								type="checkbox"
								class="size-4 shrink-0 accent-foreground"
								checked={!!role}
								aria-label={`Access to ${p.name}`}
								onchange={(e) => toggle(p.id, e.currentTarget.checked)}
							/>
							<ProjectAvatar seed={p.slug} muted={!role} class="size-5" />
							<span class="flex min-w-0 flex-col">
								<span
									class={cn('truncate text-[13px] font-medium', !role && 'text-muted-foreground')}
									>{p.name}</span
								>
								<span class="truncate font-mono text-xs text-muted-foreground/80">{p.slug}</span>
							</span>
						</label>
						<div
							class="flex h-8 shrink-0 items-center gap-0.5 rounded-md border border-input p-[3px]"
							role="radiogroup"
							aria-label={`Role on ${p.name}`}
						>
							{#each projectRoles as r (r.value)}
								{@const active = role === r.value}
								<button
									type="button"
									role="radio"
									aria-checked={active}
									disabled={!role}
									class={cn(
										segment,
										'px-2.5 text-xs',
										active
											? 'bg-accent text-foreground'
											: 'text-muted-foreground enabled:hover:text-foreground'
									)}
									onclick={() => setRole(p.id, r.value)}
								>
									{r.label}
								</button>
							{/each}
						</div>
					</li>
				{/each}
			</ul>
			<p class="text-xs text-muted-foreground/80">
				{projects.length} of {list.data.length}
				{list.data.length === 1 ? 'project' : 'projects'}
			</p>
		{/if}
	{/if}
	{#if error}<p class="text-xs/normal text-destructive">{error}</p>{/if}
</div>
