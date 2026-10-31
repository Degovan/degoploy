<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { beforeNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { ScriptRevision } from '#lib/api/types.gen.js';
	import BuiltinVariables from '#lib/components/builtin-variables.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import ProjectRuntimesSummary from '#lib/components/project-runtimes-summary.svelte';
	import ScriptEditor from '#lib/components/script-editor.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { cn } from '#lib/utils.js';
	import ArrowRightIcon from '@lucide/svelte/icons/arrow-right';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import HistoryIcon from '@lucide/svelte/icons/history';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import SaveIcon from '@lucide/svelte/icons/save';
	import Settings2Icon from '@lucide/svelte/icons/settings-2';
	import SquareTerminalIcon from '@lucide/svelte/icons/square-terminal';
	import UndoIcon from '@lucide/svelte/icons/undo-2';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));
	const script = createQuery(() => ({
		queryKey: projectKeys.script(data.id),
		queryFn: () => projectsApi.script(data.id),
		enabled: session.canOn(project.data, 'project.deploy')
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
	let cursor = $state({ line: 1, col: 1 });

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
	const managed = $derived(script.data?.source === 'managed');
	const fileName = $derived(script.data?.path.split('/').pop() || 'deploy.sh');

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
			toast.success('Revision saved');
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			saving = false;
		}
	}

	function discard() {
		if (loadedFrom !== null) draft = loadedFrom;
	}

	function restore(rev: ScriptRevision) {
		viewing = null;
		draft = rev.content;
		toast.info(`Revision #${rev.id} loaded into the editor. Save to make it current.`);
	}

	const day = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' });
	const time = new Intl.DateTimeFormat(undefined, { timeStyle: 'short' });
	const when = (rev: ScriptRevision) => {
		const d = new Date(rev.created_at);
		return `${day.format(d)} · ${time.format(d)} · ${rev.created_by || 'system'}`;
	};

	const modKey =
		typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘' : 'Ctrl';
</script>

<svelte:window
	onbeforeunload={(e) => {
		if (dirty) e.preventDefault();
	}}
/>

<PageHeader
	title="Deploy Script"
	description="Runs with bash in the working directory after the repository is updated."
>
	{#snippet aside()}
		{#if script.data}
			<span
				class="inline-flex h-6 items-center gap-1.5 rounded-full bg-muted px-2.5 text-xs [&_svg]:size-[13px] [&_svg]:text-muted-foreground"
			>
				{#if managed}<SquareTerminalIcon />Managed by degoploy{:else}<GitBranchIcon />From the
					repository{/if}
			</span>
		{/if}
	{/snippet}
	{#snippet actions()}
		{#if session.can('project.edit')}
			<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved, plus a section hash -->
			<Button
				variant="outline"
				class="h-9 px-3 text-[13px]"
				href={`${resolve('/(app)/projects/[id]/settings', { id: String(data.id) })}#deploy-script`}
			>
				<Settings2Icon class="text-muted-foreground" />
				Change source
			</Button>
			<!-- eslint-enable svelte/no-navigation-without-resolve -->
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page pt-8 pb-18">
	{#if script.isPending}
		<div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_20rem]">
			<Skeleton class="h-[32rem] rounded-xl" />
			<Skeleton class="h-48 rounded-xl" />
		</div>
	{:else if script.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{script.error.message}</Alert.Title>
		</Alert.Root>
	{:else}
		{@const s = script.data}
		<div class="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_20rem]">
			<section
				class="flex min-w-0 flex-col overflow-hidden rounded-xl border bg-card"
				aria-label="Deploy script"
			>
				<div class="flex flex-wrap items-center gap-x-2.5 gap-y-2 border-b py-3 pr-3 pl-4">
					<SquareTerminalIcon class="size-4 shrink-0 text-muted-foreground" />
					<div class="flex min-w-0 flex-1 flex-col gap-0.5">
						<span class="truncate font-mono text-[13px] font-medium">
							{fileName}{#if viewing}<span class="font-sans font-normal text-muted-foreground">
									&nbsp;· Revision #{viewing.id}</span
								>{/if}
						</span>
						<span class="truncate font-mono text-xs text-muted-foreground/80" title={s.path}
							>{s.path}</span
						>
					</div>

					<div class="ml-auto flex items-center gap-2.5">
						{#if viewing}
							<span class="flex items-center gap-1.5 text-xs text-muted-foreground">
								<LockIcon class="size-3" />Read-only
							</span>
							<Button
								variant="ghost"
								class="h-8 px-2 text-[13px] text-muted-foreground"
								onclick={() => (viewing = null)}
							>
								Back to current
							</Button>
							{#if s.editable}
								<Button class="h-8 px-3.5 text-[13px]" onclick={() => restore(viewing!)}>
									<UndoIcon />
									Restore
								</Button>
							{/if}
						{:else if s.editable}
							{#if dirty}
								<span class="flex items-center gap-1.5 px-1.5 text-xs text-muted-foreground">
									<span class="size-1.5 rounded-full bg-amber-400"></span>
									Unsaved changes
								</span>
								<Button
									variant="ghost"
									class="h-8 px-2 text-[13px] text-muted-foreground"
									onclick={discard}
									disabled={saving}
								>
									Discard
								</Button>
							{/if}
							<Button class="h-8 px-3.5 text-[13px]" onclick={save} disabled={!dirty || saving}>
								{#if saving}<LoaderIcon class="animate-spin" />{:else}<SaveIcon />{/if}
								Save revision
							</Button>
						{:else}
							<span class="flex items-center gap-1.5 text-xs text-muted-foreground">
								<LockIcon class="size-3" />Read-only
							</span>
						{/if}
					</div>
				</div>

				{#if managed || s.exists}
					<div class="h-[clamp(20rem,65svh,41rem)] bg-background">
						{#if viewing}
							{#key viewing.id}
								<ScriptEditor value={viewing.content} readonly bind:cursor />
							{/key}
						{:else}
							<ScriptEditor bind:value={draft} bind:cursor readonly={!editable} onsave={save} />
						{/if}
					</div>
				{:else}
					<div
						class="flex h-[clamp(14rem,40svh,24rem)] flex-col items-center justify-center gap-2 bg-background px-6 text-center"
					>
						<GitBranchIcon class="size-5 text-muted-foreground" />
						<p class="text-sm font-medium">Not pulled yet</p>
						<p class="max-w-sm text-[13px] text-muted-foreground">
							This script lives in the repository. It shows up here after the first pull.
						</p>
					</div>
				{/if}

				<div
					class="flex items-center gap-4 border-t px-4 py-[9px] text-xs text-muted-foreground/80"
				>
					<span class="min-w-0 flex-1 truncate">
						{#if !managed}
							Edit and commit it in the repository; degoploy reads it after each pull.
						{:else if !s.editable}
							Read-only: only admins can change deploy scripts.
						{:else}
							Runs with bash in <span class="font-mono">{project.data?.work_dir ?? '…'}</span>
						{/if}
					</span>
					{#if managed || s.exists}
						<span class="hidden shrink-0 font-mono sm:inline"
							>Ln {cursor.line}, Col {cursor.col}</span
						>
					{/if}
					{#if editable}
						<span class="hidden shrink-0 items-center gap-1.5 sm:flex">
							<kbd
								class="rounded border border-input px-1.5 py-px font-mono text-[11px] text-muted-foreground"
								>{modKey} S</kbd
							>
							to save
						</span>
					{/if}
				</div>
			</section>

			<div class="flex min-w-0 flex-col gap-5">
				{#if project.data}
					<ProjectRuntimesSummary project={project.data} />
				{/if}
				{#if managed}
					<section class="flex flex-col gap-4 rounded-xl border bg-card p-6" aria-label="Revisions">
						<h2 class="flex items-center gap-2 text-sm font-semibold">
							<HistoryIcon class="size-4 text-muted-foreground" />
							Revisions
						</h2>
						{#if revisions.isPending}
							<Skeleton class="h-20 w-full" />
						{:else if revisions.isError}
							<p class="text-[13px] text-destructive">{revisions.error.message}</p>
						{:else}
							<ol class="-mr-2 max-h-[22rem] [scrollbar-width:thin] overflow-y-auto pr-2">
								{#each revisions.data as rev, i (rev.id)}
									{@const current = i === 0}
									{@const shown = current ? !viewing : viewing?.id === rev.id}
									{@const last = i === revisions.data.length - 1}
									<li class="flex gap-3">
										<div class="flex w-2.5 shrink-0 flex-col items-center gap-1 pt-[5px]">
											<span
												class={cn(
													'size-[9px] shrink-0 rounded-full border-[1.5px]',
													current
														? 'border-foreground bg-foreground'
														: 'border-muted-foreground/80 bg-background'
												)}
											></span>
											{#if !last}<span class="w-px flex-1 bg-input"></span>{/if}
										</div>
										<div class={cn('flex min-w-0 flex-1 flex-col gap-1', !last && 'pb-[18px]')}>
											<div class="flex min-h-[22px] items-center gap-2">
												<button
													type="button"
													class={cn(
														'text-left text-[13px] font-medium underline-offset-2 hover:underline',
														!current && shown && 'underline'
													)}
													onclick={() => (viewing = current ? null : rev)}
													aria-pressed={shown}
													title={current ? 'Show the current script' : 'Preview this revision'}
												>
													Revision #{rev.id}
												</button>
												{#if current}
													<span
														class="inline-flex h-[22px] items-center gap-1.5 rounded-full bg-emerald-400/10 px-2 text-xs font-medium text-emerald-600 dark:text-emerald-400"
													>
														<span class="size-1.5 rounded-full bg-current"></span>Current
													</span>
												{:else if s.editable}
													<button
														type="button"
														class="ml-auto text-xs font-medium text-muted-foreground hover:text-foreground"
														onclick={() => restore(rev)}
													>
														Restore
													</button>
												{/if}
											</div>
											<span class="text-xs text-muted-foreground/80">{when(rev)}</span>
										</div>
									</li>
								{/each}
							</ol>
						{/if}
					</section>
				{/if}

				<BuiltinVariables>
					<a
						href={resolve('/(app)/projects/[id]/environment', { id: String(data.id) })}
						class="-mx-2 -mb-2 flex items-center justify-between rounded-md px-2 py-1.5 text-[13px] text-muted-foreground hover:bg-accent hover:text-foreground"
					>
						Project variables & secrets
						<ArrowRightIcon class="size-3.5" />
					</a>
				</BuiltinVariables>
			</div>
		</div>
	{/if}
</div>
