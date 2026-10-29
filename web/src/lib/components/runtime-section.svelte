<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { projectKeys } from '#lib/api/projects.js';
	import {
		formatProgress,
		formatSize,
		lineOf,
		progressPercent,
		runtimeExamples,
		runtimeKeys,
		runtimesApi
	} from '#lib/api/runtimes.js';
	import type { RuntimeProgress, RuntimeStatus, RuntimeVersion } from '#lib/api/types.gen.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import BracesIcon from '@lucide/svelte/icons/braces';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import HexagonIcon from '@lucide/svelte/icons/hexagon';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PinIcon from '@lucide/svelte/icons/pin';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import ServerIcon from '@lucide/svelte/icons/server';
	import StarIcon from '@lucide/svelte/icons/star';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import XIcon from '@lucide/svelte/icons/x';

	// One runtime on the Runtimes page: its switch, installed versions,
	// downloads in progress, default version and installs.
	let {
		runtime: rt,
		onbusy
	}: {
		runtime: RuntimeStatus;
		/** Called with +1 when this section starts a request that installs, -1 when it ends. */
		onbusy: (delta: 1 | -1) => void;
	} = $props();

	const queryClient = useQueryClient();
	const releases = createQuery(() => ({
		queryKey: runtimeKeys.releases(rt.name),
		queryFn: () => runtimesApi.releases(rt.name),
		staleTime: 60 * 60 * 1000,
		retry: false
	}));
	const ex = $derived(runtimeExamples[rt.name] ?? { line: '1', exact: '1.0.0', files: [] });
	const line = (version: string) => lineOf(rt.name, version);
	const Icon = $derived(rt.name === 'php' ? BracesIcon : HexagonIcon);

	function apply(next: RuntimeStatus) {
		queryClient.setQueryData<RuntimeStatus[]>(runtimeKeys.all, (all) =>
			all?.map((r) => (r.name === next.name ? next : r))
		);
		queryClient.invalidateQueries({ queryKey: runtimeKeys.releases(rt.name) });
	}

	// Lines worth offering: Node.js's newest LTS lines (and the current one
	// while it is newer than all of them); PHP's newest minor versions.
	const lines = $derived.by(() => {
		const all = releases.data ?? [];
		if (!all.some((r) => r.lts)) return all.slice(0, 4);
		const lts = all.filter((r) => r.lts).slice(0, 3);
		const current = all.find((r) => !r.lts);
		const newer =
			current && (!lts[0] || Number(line(current.version)) > Number(line(lts[0].version)));
		return newer ? [current, ...lts] : lts;
	});

	let installing = $state<string | null>(null);
	let spec = $state('');
	let specError = $state('');
	async function install(s: string) {
		s = s.trim();
		specError = '';
		if (!s) {
			specError = `Enter a version, such as ${ex.line}.`;
			return;
		}
		installing = s;
		onbusy(1);
		try {
			const before = new Set(rt.installed.map((v) => v.version));
			const next = await runtimesApi.install(rt.name, s);
			apply(next);
			const added = next.installed.find((v) => !before.has(v.version));
			toast.success(
				added ? `${rt.title} ${added.version} installed` : `${rt.title} ${s} was already installed`
			);
			if (s === spec.trim()) spec = '';
		} catch (err) {
			if (err instanceof ApiError && err.field === 'spec') specError = err.message;
			else if (!(err instanceof ApiError && err.code === 'canceled'))
				toast.error(errorMessage(err));
		} finally {
			installing = null;
			onbusy(-1);
		}
	}

	let updating = $state(false);
	async function update() {
		updating = true;
		onbusy(1);
		try {
			const res = await runtimesApi.update(rt.name);
			apply(res.status);
			if (res.changes.length === 0)
				toast.success(`Every downloaded ${rt.title} version is up to date`);
			else
				toast.success(
					`Updated ${res.changes.map((c) => `${c.from} → ${c.to}`).join(', ')}`,
					res.changes.some((c) => !c.removed)
						? { description: 'Versions a project pins were kept.' }
						: undefined
				);
		} catch (err) {
			if (!(err instanceof ApiError && err.code === 'canceled')) toast.error(errorMessage(err));
		} finally {
			updating = false;
			onbusy(-1);
		}
	}

	// Cancel stops the download wherever it was started; a deployment that was
	// waiting for it fails.
	let canceling = $state<string | null>(null);
	async function cancelInstall(version: string) {
		canceling = version;
		try {
			await runtimesApi.cancel(rt.name, version);
			await queryClient.invalidateQueries({ queryKey: runtimeKeys.all, exact: true });
			toast(`Install of ${rt.title} ${version} canceled`);
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			canceling = null;
		}
	}

	let defaulting = $state<string | null>(null);
	async function setDefault(next: string) {
		defaulting = next;
		try {
			apply(await runtimesApi.setDefault(rt.name, next));
			// Project settings describe what Auto falls back to.
			queryClient.invalidateQueries({ queryKey: projectKeys.all });
			toast.success(next ? `${rt.title} ${next} is the default` : 'No default version');
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			defaulting = null;
		}
	}

	let switching = $state(false);
	async function setEnabled(on: boolean) {
		switching = true;
		try {
			apply(await runtimesApi.setEnabled(rt.name, on));
			toast.success(on ? `${rt.title} is on` : `${rt.title} is off for every project`);
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			switching = false;
		}
	}

	let removing = $state<RuntimeVersion | null>(null);
	let removeBusy = $state(false);
	async function remove() {
		if (!removing) return;
		removeBusy = true;
		try {
			apply(await runtimesApi.remove(rt.name, removing.version));
			toast.success(`${rt.title} ${removing.version} removed`);
			removing = null;
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			removeBusy = false;
		}
	}

	const chip =
		'inline-flex h-[22px] shrink-0 items-center gap-[5px] rounded-[5px] border px-[7px] text-xs';
	const hint = 'text-xs/normal text-muted-foreground/80';
</script>

{#snippet bar(p: RuntimeProgress)}
	{@const pct = p.phase === 'unpacking' ? 100 : progressPercent(p)}
	<span
		class="block h-1.5 w-full overflow-hidden rounded-full bg-muted"
		role="progressbar"
		aria-label={`Installing ${rt.title} ${p.version}`}
		aria-valuemin={0}
		aria-valuemax={100}
		aria-valuenow={pct ?? undefined}
	>
		<span
			class={cn(
				'block h-full rounded-full bg-foreground transition-[width] duration-500',
				(pct === null || p.phase === 'unpacking') && 'animate-pulse'
			)}
			style:width={`${pct ?? 100}%`}
		></span>
	</span>
{/snippet}

<section
	id={rt.name}
	class="scroll-mt-32 overflow-hidden rounded-xl border bg-card"
	aria-labelledby={`${rt.name}-title`}
>
	<div class="flex flex-wrap items-start justify-between gap-4 border-b p-6">
		<div class="flex min-w-0 flex-1 items-start gap-4">
			<span
				class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
			>
				<Icon class="size-[18px]" />
			</span>
			<div class="flex min-w-0 flex-col gap-1.5">
				<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
					<h2 id={`${rt.name}-title`} class="text-xl font-semibold tracking-[-0.4px]">
						{rt.title}
					</h2>
					{#if rt.installed.length}
						<span class="text-[13px] text-muted-foreground">
							{rt.installed.length}
							{rt.installed.length === 1 ? 'version' : 'versions'}{rt.total_size
								? ` · ${formatSize(rt.total_size)}`
								: ''}
						</span>
					{/if}
				</div>
				<p class="text-sm/normal text-muted-foreground">
					Projects on Auto use it when their repository has
					{#each ex.files as f, i (f)}<span class="font-mono text-[13px]">{f}</span>{i <
						ex.files.length - 2
							? ', '
							: i === ex.files.length - 2
								? ' or '
								: ''}{/each}.
				</p>
			</div>
		</div>
		<div class="flex items-center gap-2">
			<Label for={`${rt.name}-on`} class="text-[13px] text-muted-foreground"
				>{rt.enabled ? 'On' : 'Off'}</Label
			>
			<Switch
				id={`${rt.name}-on`}
				checked={rt.enabled}
				disabled={switching}
				onCheckedChange={setEnabled}
				class="data-[size=default]:h-5 data-[size=default]:w-[34px]"
			/>
		</div>
	</div>

	{#if !rt.enabled}
		<div class="flex items-start gap-2.5 border-b bg-background px-6 py-3.5 text-[13px]">
			<InfoIcon class="mt-0.5 size-3.5 shrink-0 text-muted-foreground/80" />
			<span class="text-muted-foreground">
				{rt.title} is off: no deployment uses it, whatever the project settings say. Scripts get the server's
				own, if there is one.
			</span>
		</div>
	{/if}

	{#if rt.installing.length}
		<ul class="divide-y border-b" aria-label={`Installing ${rt.title}`}>
			{#each rt.installing as p (p.version)}
				<li class="flex flex-col gap-2.5 px-6 py-4">
					<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
						<LoaderIcon class="size-3.5 animate-spin text-muted-foreground" />
						<span class="font-mono text-sm font-semibold">{p.version}</span>
						<span class="flex-1 text-[13px] text-muted-foreground">
							{p.phase === 'unpacking' ? 'Unpacking…' : `Downloading · ${formatProgress(p)}`}
						</span>
						<Button
							variant="ghost"
							class="-my-1 h-7 px-2 text-[13px] text-muted-foreground hover:text-destructive"
							disabled={canceling === p.version}
							title="Stop the download. A deployment waiting for it fails."
							onclick={() => cancelInstall(p.version)}
						>
							{#if canceling === p.version}<LoaderIcon class="animate-spin" />{:else}<XIcon />{/if}
							Cancel
						</Button>
					</div>
					{@render bar(p)}
				</li>
			{/each}
		</ul>
	{/if}

	{#if rt.installed.length === 0 && rt.installing.length === 0}
		<div class="flex flex-col items-center gap-2 px-6 py-10 text-center">
			<Icon class="size-6 text-muted-foreground/60" />
			<p class="text-sm font-medium">No {rt.title} versions yet</p>
			<p class="max-w-md text-[13px] text-muted-foreground">
				A deployment installs the version its project asks for the first time it needs it. You can
				also install one below.
			</p>
		</div>
	{:else if rt.installed.length}
		<ul class="divide-y" aria-label={`${rt.title} versions`}>
			{#each rt.installed as v (v.version)}
				<li class="flex flex-wrap items-center gap-x-6 gap-y-3 px-6 py-4">
					<div class="flex min-w-0 flex-1 flex-col gap-1.5">
						<div class="flex flex-wrap items-center gap-2">
							<span class="font-mono text-sm font-semibold">{v.version}</span>
							{#if v.system}
								<span
									class={cn(chip, 'border-input bg-muted text-muted-foreground')}
									title="Installed with the system's package manager; degoploy uses it as it is."
									><ServerIcon class="size-3" />System</span
								>
							{/if}
							{#if v.lts}
								<span class={cn(chip, 'border-input bg-muted text-muted-foreground')}
									>LTS · {v.lts}</span
								>
							{/if}
							{#if v.default}
								<span
									class={cn(
										chip,
										'border-emerald-400/25 bg-emerald-400/[0.08] text-emerald-600 dark:text-emerald-400'
									)}><StarIcon class="size-3" />Default</span
								>
							{/if}
						</div>
						<p class="text-[13px] text-muted-foreground">
							{#if v.used_by.length}
								Used by {v.used_by.join(', ')}
							{:else}
								Not used by the latest deployments
							{/if}
							{#if v.system}
								<span class="font-mono text-xs text-muted-foreground/60"> · {v.path}</span>
							{:else}
								<span class="text-muted-foreground/60"> · {formatSize(v.size)}</span>
								{#if v.installed_at}
									<span class="text-muted-foreground/60" title={formatDateTime(v.installed_at)}>
										· installed {timeAgo(v.installed_at)}</span
									>
								{/if}
							{/if}
						</p>
						{#if v.pinned.length}
							<p class={cn(hint, 'flex items-center gap-1.5')}>
								<PinIcon class="size-3" />Pinned by {v.pinned.join(', ')}{v.system
									? ''
									: '; Update all keeps it'}.
							</p>
						{/if}
					</div>
					<div class="flex shrink-0 items-center gap-2">
						{#if !v.default}
							<Button
								variant="ghost"
								class="h-8 px-2.5 text-[13px]"
								disabled={defaulting !== null}
								onclick={() => setDefault(line(v.version))}
							>
								{#if defaulting === line(v.version)}<LoaderIcon class="animate-spin" />{/if}
								Make default
							</Button>
						{/if}
						{#if v.system}
							<span class="size-8" aria-hidden="true"></span>
						{:else}
							<Button
								variant="ghost"
								size="icon"
								class="size-8 text-muted-foreground hover:text-destructive"
								aria-label={`Remove ${rt.title} ${v.version}`}
								onclick={() => (removing = v)}
							>
								<Trash2Icon class="size-4" />
							</Button>
						{/if}
					</div>
				</li>
			{/each}
		</ul>
	{/if}

	<div class="flex flex-col gap-4 border-t px-6 py-5">
		<div class="flex flex-wrap items-center justify-between gap-2">
			<h3 class="text-sm font-medium">Install a version</h3>
			{#if rt.installed.some((v) => !v.system)}
				<Button
					variant="outline"
					class="h-8 px-3 text-[13px]"
					disabled={updating}
					onclick={update}
					title="Install the latest patch of each downloaded line and remove the older one"
				>
					{#if updating}<LoaderIcon class="animate-spin" />{:else}<RefreshCwIcon />{/if}
					Update all
				</Button>
			{/if}
		</div>
		{#if lines.length}
			<div
				class={cn(
					'grid gap-2 sm:grid-cols-2',
					lines.length === 3 ? 'lg:grid-cols-3' : 'lg:grid-cols-4'
				)}
			>
				{#each lines as r (r.version)}
					{@const l = line(r.version)}
					{@const has = rt.installed.some((v) => line(v.version) === l)}
					{@const prog = rt.installing.find((p) => line(p.version) === l)}
					<button
						type="button"
						class="flex flex-col gap-1 rounded-lg border border-input bg-background p-3.5 text-left transition-colors enabled:hover:border-muted-foreground/60"
						disabled={has || installing !== null}
						onclick={() => install(l)}
					>
						<span class="flex items-center justify-between gap-2">
							<span class="text-sm font-medium">{rt.title} {l}</span>
							{#if has}
								<CheckIcon class="size-4 text-emerald-400" />
							{:else if prog || installing === l}
								<span class="flex items-center gap-1.5 text-xs text-muted-foreground">
									{#if prog?.phase === 'downloading' && progressPercent(prog) !== null}
										{progressPercent(prog)}%
									{/if}
									<LoaderIcon class="size-4 animate-spin" />
								</span>
							{:else}
								<DownloadIcon class="size-4 text-muted-foreground/70" />
							{/if}
						</span>
						<span class="font-mono text-xs text-muted-foreground">{r.version}</span>
						<span class="text-xs text-muted-foreground/80">
							{r.lts ? `LTS · ${r.lts}` : rt.name === 'node' ? 'Current' : 'Newest patch'}{has
								? ' · installed'
								: ''}
						</span>
						{#if prog}
							<span class="pt-1.5">{@render bar(prog)}</span>
						{/if}
					</button>
				{/each}
			</div>
		{:else if releases.isError}
			<p class={cn(hint, 'flex items-center gap-1.5')}>
				<CircleAlertIcon class="size-3.5 text-amber-500" />
				The release list from {rt.mirror} could not be loaded: {releases.error.message}
			</p>
		{:else if releases.isPending}
			<Skeleton class="h-[76px] rounded-lg" />
		{/if}

		<form
			class="flex flex-col gap-2"
			onsubmit={(e) => {
				e.preventDefault();
				install(spec);
			}}
			novalidate
		>
			<Label for={`${rt.name}-spec`} class="text-[13px] text-muted-foreground">Other version</Label>
			<div class="flex flex-col gap-2 sm:flex-row">
				<Input
					id={`${rt.name}-spec`}
					bind:value={spec}
					placeholder={`${ex.line} or ${ex.exact}`}
					spellcheck={false}
					autocomplete="off"
					class="h-10 bg-background font-mono text-[13px] sm:max-w-[280px] dark:bg-background"
					aria-invalid={!!specError}
				/>
				<Button
					type="submit"
					variant="outline"
					class="h-10 px-3 text-[13px]"
					disabled={installing !== null || !spec.trim()}
				>
					{#if installing !== null && installing === spec.trim()}<LoaderIcon
							class="animate-spin"
						/>{:else}<DownloadIcon />{/if}
					Install
				</Button>
			</div>
			{#if specError}
				<p class="text-xs/normal text-destructive" role="alert">{specError}</p>
			{:else}
				<p class={hint}>
					{#if rt.verified}
						Downloads the official {rt.platform} build from
						<span class="font-mono">{rt.mirror}</span> and checks its SHA256 sum.
					{:else}
						Downloads a static {rt.platform} build from <span class="font-mono">{rt.mirror}</span>,
						checks the SHA256 it publishes for recent builds, and runs it to confirm the version.
					{/if}
				</p>
			{/if}
		</form>
	</div>

	<div
		class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t bg-background px-6 py-3"
	>
		<p class="flex items-start gap-2 text-[13px] text-muted-foreground">
			<InfoIcon class="mt-0.5 size-3.5 shrink-0 text-muted-foreground/80" />
			{#if rt.default}
				<span>
					Default: <span class="font-medium text-foreground">{rt.title} {rt.default}</span>, for
					projects on Auto whose repository names no version.
				</span>
			{:else}
				<span>
					No default version: projects on Auto whose repository names no version use the server's
					own
					{rt.name}, if there is one.
				</span>
			{/if}
		</p>
		{#if rt.default}
			<Button
				variant="ghost"
				class="h-7 px-2 text-[13px] text-muted-foreground"
				disabled={defaulting !== null}
				onclick={() => setDefault('')}>Clear default</Button
			>
		{/if}
	</div>
</section>

<AlertDialog.Root
	open={removing !== null}
	onOpenChange={(open) => {
		if (!open && !removeBusy) removing = null;
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Remove {rt.title} {removing?.version}?</AlertDialog.Title>
			<AlertDialog.Description>
				{#if removing?.used_by.length}
					{removing.used_by.join(', ')} deployed with it. A project that still asks for it installs it
					again on its next deployment.
				{:else}
					No recent deployment used it.
				{/if}
				{#if rt.name === 'node'}Packages installed globally with it are removed too.{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={removeBusy}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" disabled={removeBusy} onclick={remove}>
				{#if removeBusy}<LoaderIcon class="animate-spin" />{/if}
				Remove
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
