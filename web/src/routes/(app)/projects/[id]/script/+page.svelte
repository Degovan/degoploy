<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { beforeNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { ScriptRevision } from '#lib/api/types.gen.js';
	import ScriptEditor from '#lib/components/script-editor.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import SaveIcon from '@lucide/svelte/icons/save';
	import UndoIcon from '@lucide/svelte/icons/undo-2';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const script = createQuery(() => ({
		queryKey: projectKeys.script(data.id),
		queryFn: () => projectsApi.script(data.id),
		enabled: session.can('project.deploy')
	}));
	const revisions = createQuery(() => ({
		queryKey: projectKeys.revisions(data.id),
		queryFn: () => projectsApi.revisions(data.id),
		enabled: script.data?.source === 'managed'
	}));

	let draft = $state('');
	let loadedFrom = $state<string | null>(null);
	let saving = $state(false);
	let viewing = $state<ScriptRevision | null>(null);

	// Seed the editor once per fetched version, without clobbering local edits on refetch.
	$effect(() => {
		const content = script.data?.content;
		if (content !== undefined && loadedFrom === null) {
			draft = content;
			loadedFrom = content;
		}
	});

	const dirty = $derived(loadedFrom !== null && draft !== loadedFrom);
	const editable = $derived(!!script.data?.editable && !viewing);

	beforeNavigate((nav) => {
		if (dirty && !confirm('Discard unsaved changes to the deploy script?')) nav.cancel();
	});

	async function save() {
		if (!editable || !dirty || saving) return;
		saving = true;
		try {
			const next = await projectsApi.saveScript(data.id, draft);
			queryClient.setQueryData(projectKeys.script(data.id), next);
			loadedFrom = next.content;
			draft = next.content;
			await queryClient.invalidateQueries({ queryKey: projectKeys.revisions(data.id) });
			toast.success('Script saved');
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			saving = false;
		}
	}

	function restore(rev: ScriptRevision) {
		viewing = null;
		draft = rev.content;
		toast.info(`Revision #${rev.id} loaded into the editor. Save to make it current.`);
	}

	const fmt = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });
</script>

<svelte:window
	onbeforeunload={(e) => {
		if (dirty) e.preventDefault();
	}}
/>

{#if script.isPending}
	<Skeleton class="h-96 w-full rounded-xl" />
{:else if script.isError}
	<Alert.Root variant="destructive">
		<CircleAlertIcon />
		<Alert.Title>{script.error.message}</Alert.Title>
	</Alert.Root>
{:else}
	{@const s = script.data}
	<div class="grid gap-6 xl:grid-cols-[1fr_18rem]">
		<Card.Root class="min-w-0">
			<Card.Header>
				<Card.Title class="flex items-center gap-2">
					Deploy script
					{#if viewing}
						<Badge variant="secondary">Revision #{viewing.id}</Badge>
					{:else if dirty}
						<Badge variant="outline">Unsaved</Badge>
					{/if}
				</Card.Title>
				<Card.Description class="font-mono text-xs break-all">{s.path}</Card.Description>
				{#if s.editable}
					<Card.Action class="flex gap-2">
						{#if viewing}
							<Button variant="outline" size="sm" onclick={() => (viewing = null)}
								>Back to editor</Button
							>
							<Button size="sm" onclick={() => restore(viewing!)}>
								<UndoIcon />
								Restore
							</Button>
						{:else}
							<Button size="sm" onclick={save} disabled={!dirty || saving}>
								{#if saving}<LoaderIcon class="animate-spin" />{:else}<SaveIcon />{/if}
								Save
							</Button>
						{/if}
					</Card.Action>
				{/if}
			</Card.Header>
			<Card.Content class="flex flex-col gap-3">
				{#if s.source === 'repo'}
					<Alert.Root>
						<GitBranchIcon />
						<Alert.Title>This script lives in the repository</Alert.Title>
						<Alert.Description>
							Edit and commit it in the repo; degoploy reads it from the working directory after
							each pull.
							{#if !s.exists}It has not been pulled yet, so there is nothing to show.{/if}
						</Alert.Description>
					</Alert.Root>
				{/if}
				{#if s.source === 'managed' || s.exists}
					{#if viewing}
						{#key viewing.id}
							<ScriptEditor value={viewing.content} readonly />
						{/key}
					{:else}
						<ScriptEditor bind:value={draft} readonly={!editable} onsave={save} />
					{/if}
				{/if}
				{#if s.editable && !viewing}
					<p class="text-xs text-muted-foreground">
						Runs with <code>bash</code> in the working directory. Press <kbd>Ctrl</kbd>+<kbd>S</kbd> to
						save.
					</p>
				{/if}
			</Card.Content>
		</Card.Root>

		{#if s.source === 'managed'}
			<Card.Root class="h-fit">
				<Card.Header>
					<Card.Title class="flex items-center gap-2 text-base">
						<HistoryIcon class="size-4" />
						Revisions
					</Card.Title>
				</Card.Header>
				<Card.Content class="flex flex-col gap-1 px-2">
					{#if revisions.isPending}
						<Skeleton class="h-24 w-full" />
					{:else if revisions.data}
						{#each revisions.data as rev, i (rev.id)}
							<button
								type="button"
								class={cn(
									'flex flex-col items-start gap-0.5 rounded-md px-3 py-2 text-left text-sm hover:bg-muted',
									viewing?.id === rev.id && 'bg-muted'
								)}
								onclick={() => (viewing = i === 0 ? null : rev)}
							>
								<span class="flex items-center gap-2 font-medium">
									#{rev.id}
									{#if i === 0}<Badge variant="secondary">Current</Badge>{/if}
								</span>
								<span class="text-xs text-muted-foreground">
									{fmt.format(new Date(rev.created_at))} · {rev.created_by || 'system'}
								</span>
							</button>
						{/each}
					{/if}
				</Card.Content>
			</Card.Root>
		{/if}
	</div>

	{#if !s.editable && s.source === 'managed'}
		<p class="text-sm text-muted-foreground">
			Read-only: only admins can change deploy scripts.
			<a class="underline" href={resolve('/(app)/projects/[id]', { id: String(data.id) })}
				>Back to overview</a
			>
		</p>
	{/if}
{/if}
