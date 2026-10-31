<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { cacheDeployment, deploymentKeys, shortRef } from '#lib/api/deployments.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { Deployment, Project, Release } from '#lib/api/types.gen.js';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import ArchiveIcon from '@lucide/svelte/icons/archive';
	import UndoIcon from '@lucide/svelte/icons/undo-2';

	// The releases kept on the server; current points at the live one.
	let { project: p, active }: { project: Project; active: Deployment | null } = $props();
	const queryClient = useQueryClient();

	const releases = createQuery(() => ({
		queryKey: projectKeys.releases(p.id),
		queryFn: () => projectsApi.releases(p.id)
	}));
	const live = $derived(releases.data?.find((r) => r.live)?.id ?? null);
	const href = (did: number) =>
		resolve('/(app)/projects/[id]/deployments/[did]', { id: String(p.id), did: String(did) });

	async function activate(r: Release) {
		try {
			const d = await projectsApi.activateRelease(p.id, r.id);
			cacheDeployment(queryClient, d);
			await queryClient.invalidateQueries({ queryKey: deploymentKeys.list(p.id) });
			toast.success(`Switching to release #${r.id} as deployment #${d.id}`);
			await goto(href(d.id));
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
</script>

<section class="flex flex-col gap-4" aria-labelledby="releases">
	<SectionHeading
		title="Releases"
		id="releases"
		description={`${p.work_dir}/current points at the live release. The newest ${p.keep_releases} stay on the server.`}
	/>
	<div class="overflow-hidden rounded-xl border bg-card">
		{#if releases.isPending}
			<Skeleton class="h-24 w-full" />
		{:else if releases.isError}
			<p class="p-6 text-sm text-destructive">{releases.error.message}</p>
		{:else if releases.data.length === 0}
			<div class="flex flex-col items-center gap-2 px-6 py-10 text-center">
				<ArchiveIcon class="mb-1 size-5 text-muted-foreground" />
				<p class="font-medium">No releases yet</p>
				<p class="max-w-md text-sm text-muted-foreground">
					The next deployment builds the first one and points <code>current</code> at it when its script
					succeeds.
				</p>
			</div>
		{:else}
			<ul class="divide-y">
				{#each releases.data as r (r.id)}
					{@const d = r.deployment}
					<li class="flex items-center gap-4 px-5 py-3.5 text-sm">
						<a
							href={href(r.id)}
							class="w-14 shrink-0 font-mono text-muted-foreground hover:underline">#{r.id}</a
						>
						<div class="flex min-w-0 flex-1 flex-col gap-0.5">
							<span class="truncate">
								{d?.commit_message.split('\n')[0] || (d ? shortRef(d.ref) : 'Deployment pruned')}
							</span>
							{#if d}
								<span class="truncate text-xs text-muted-foreground">
									<span class="font-mono"
										>{shortRef(d.ref)}{d.commit_sha ? ` @ ${d.commit_sha.slice(0, 7)}` : ''}</span
									>
									· <span title={formatDateTime(d.queued_at)}>{shortAgo(d.queued_at)}</span>
								</span>
							{/if}
						</div>
						{#if r.live}
							<span
								class="shrink-0 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-400"
								>Live</span
							>
						{:else if session.can('project.deploy')}
							<ConfirmButton
								size="sm"
								title={`Switch to release #${r.id}?`}
								description={`current moves from release #${live ?? '–'} to #${r.id} at once${p.after_activate ? ', then the after-activate commands run' : ''}. Database migrations the newer release ran are not undone.`}
								confirmLabel={live !== null && r.id < live ? 'Roll back' : 'Switch'}
								disabled={!!active}
								onconfirm={() => activate(r)}
							>
								<UndoIcon />
								{live !== null && r.id < live ? 'Roll back' : 'Switch'}
							</ConfirmButton>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</section>
