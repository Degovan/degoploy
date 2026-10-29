<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import {
		formatProgress,
		formatSize,
		majorOf,
		nodeApi,
		nodeKeys,
		progressPercent
	} from '#lib/api/node.js';
	import { projectKeys } from '#lib/api/projects.js';
	import type { NodeProgress, NodeStatus, NodeVersion } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import { formatDateTime, timeAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CheckIcon from '@lucide/svelte/icons/check';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import DownloadIcon from '@lucide/svelte/icons/download';
	import HexagonIcon from '@lucide/svelte/icons/hexagon';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PinIcon from '@lucide/svelte/icons/pin';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import StarIcon from '@lucide/svelte/icons/star';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import XIcon from '@lucide/svelte/icons/x';

	const queryClient = useQueryClient();
	// Installs and updates started on this page. Declared before the status
	// query, which reads them as soon as it is created.
	let installing = $state<string | null>(null);
	let updating = $state(false);
	// While anything installs (from here, the CLI or a deployment), poll for its
	// progress. busy is read here, not in refetchInterval, so the options
	// change (and polling starts) as soon as this page starts an install.
	const status = createQuery(() => {
		const busy = installing !== null || updating;
		return {
			queryKey: nodeKeys.status,
			queryFn: nodeApi.status,
			refetchInterval: (q) => (busy || q.state.data?.installing.length ? 500 : false)
		};
	});
	const releases = createQuery(() => ({
		queryKey: nodeKeys.releases,
		queryFn: nodeApi.releases,
		staleTime: 60 * 60 * 1000,
		retry: false
	}));

	function apply(next: NodeStatus) {
		queryClient.setQueryData(nodeKeys.status, next);
		queryClient.invalidateQueries({ queryKey: nodeKeys.releases });
	}

	// The major lines worth offering: the newest LTS lines, and the current
	// line while it is newer than every LTS one (older odd lines are end of life).
	const lines = $derived.by(() => {
		const all = releases.data ?? [];
		const lts = all.filter((r) => r.lts).slice(0, 3);
		const current = all.find((r) => !r.lts);
		const newer =
			current && (!lts[0] || Number(majorOf(current.version)) > Number(majorOf(lts[0].version)));
		return newer ? [current, ...lts] : lts;
	});

	let spec = $state('');
	let specError = $state('');
	async function install(s: string) {
		s = s.trim();
		specError = '';
		if (!s) {
			specError = 'Enter a version, such as 22 or lts.';
			return;
		}
		installing = s;
		try {
			const before = new Set(status.data?.installed.map((v) => v.version));
			const next = await nodeApi.install(s);
			apply(next);
			const added = next.installed.find((v) => !before.has(v.version));
			toast.success(
				added ? `Node.js ${added.version} installed` : `Node.js ${s} was already installed`
			);
			if (s === spec.trim()) spec = '';
		} catch (err) {
			if (err instanceof ApiError && err.field === 'spec') specError = err.message;
			else if (err instanceof ApiError && err.code === 'canceled') return;
			else toast.error(errorMessage(err));
		} finally {
			installing = null;
		}
	}

	async function update() {
		updating = true;
		try {
			const res = await nodeApi.update();
			apply(res.status);
			if (res.changes.length === 0) toast.success('Every version is up to date');
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
		}
	}

	// Cancel stops the download wherever it was started; a deployment that was
	// waiting for it fails.
	let canceling = $state<string | null>(null);
	async function cancelInstall(version: string) {
		canceling = version;
		try {
			await nodeApi.cancel(version);
			await queryClient.invalidateQueries({ queryKey: nodeKeys.status });
			toast(`Install of Node.js ${version} canceled`);
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
			apply(await nodeApi.setDefault(next));
			// Project settings describe what Auto falls back to.
			queryClient.invalidateQueries({ queryKey: projectKeys.all });
			toast.success(next ? `Node.js ${next} is the default` : 'No default version');
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			defaulting = null;
		}
	}

	let removing = $state<NodeVersion | null>(null);
	let removeBusy = $state(false);
	async function remove() {
		if (!removing) return;
		removeBusy = true;
		try {
			apply(await nodeApi.remove(removing.version));
			toast.success(`Node.js ${removing.version} removed`);
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

{#snippet bar(p: NodeProgress)}
	{@const pct = p.phase === 'unpacking' ? 100 : progressPercent(p)}
	<span
		class="block h-1.5 w-full overflow-hidden rounded-full bg-muted"
		role="progressbar"
		aria-label={`Installing ${p.version}`}
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

<PageHeader
	title="Node.js"
	description="Versions degoploy installs for deploy scripts. Each project picks one in its settings or with an .nvmrc file."
>
	{#snippet aside()}
		{#if status.data?.installed.length}
			{@const n = status.data.installed.length}
			<span class="text-[13px] text-muted-foreground">
				{n}
				{n === 1 ? 'version' : 'versions'} · {formatSize(status.data.total_size)}
			</span>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page flex flex-col gap-6 pt-8 pb-24">
	{#if status.isError}
		<Alert.Root variant="destructive">
			<CircleAlertIcon />
			<Alert.Title>{status.error.message}</Alert.Title>
		</Alert.Root>
	{:else if !status.data}
		<Skeleton class="h-72 rounded-xl" />
		<Skeleton class="h-56 rounded-xl" />
	{:else}
		{@const s = status.data}
		<section class="overflow-hidden rounded-xl border bg-card" aria-labelledby="installed-title">
			<div class="flex flex-wrap items-start justify-between gap-4 border-b p-6">
				<div class="flex items-start gap-4">
					<span
						class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
					>
						<HexagonIcon class="size-[18px]" />
					</span>
					<div class="flex flex-col gap-1.5">
						<h2 id="installed-title" class="text-xl font-semibold tracking-[-0.4px]">
							Installed versions
						</h2>
						<p class="text-sm/normal text-muted-foreground">
							Deployments use the newest installed version that fits what the project asks for.
						</p>
					</div>
				</div>
				{#if s.installed.length}
					<Button
						variant="outline"
						class="h-8 px-3 text-[13px]"
						disabled={updating}
						onclick={update}
						title="Install the latest patch of each line and remove the older one"
					>
						{#if updating}<LoaderIcon class="animate-spin" />{:else}<RefreshCwIcon />{/if}
						Update all
					</Button>
				{/if}
			</div>

			{#if s.installing.length}
				<ul class="divide-y border-b" aria-label="Installing">
					{#each s.installing as p (p.version)}
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
									{#if canceling === p.version}<LoaderIcon class="animate-spin" />{:else}<XIcon
										/>{/if}
									Cancel
								</Button>
							</div>
							{@render bar(p)}
						</li>
					{/each}
				</ul>
			{/if}

			{#if s.installed.length === 0 && s.installing.length === 0}
				<div class="flex flex-col items-center gap-2 px-6 py-12 text-center">
					<HexagonIcon class="size-6 text-muted-foreground/60" />
					<p class="text-sm font-medium">No versions installed yet</p>
					<p class="max-w-md text-[13px] text-muted-foreground">
						A deployment installs the version its project asks for the first time it needs it. You
						can also install one below.
					</p>
				</div>
			{:else if s.installed.length}
				<ul class="divide-y">
					{#each s.installed as v (v.version)}
						<li class="flex flex-wrap items-center gap-x-6 gap-y-3 px-6 py-4">
							<div class="flex min-w-0 flex-1 flex-col gap-1.5">
								<div class="flex flex-wrap items-center gap-2">
									<span class="font-mono text-sm font-semibold">{v.version}</span>
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
									<span class="text-muted-foreground/60"> · {formatSize(v.size)}</span>
									{#if v.installed_at}
										<span class="text-muted-foreground/60" title={formatDateTime(v.installed_at)}>
											· installed {timeAgo(v.installed_at)}</span
										>
									{/if}
								</p>
								{#if v.pinned.length}
									<p class={cn(hint, 'flex items-center gap-1.5')}>
										<PinIcon class="size-3" />Pinned by {v.pinned.join(', ')}; Update all keeps it.
									</p>
								{/if}
							</div>
							<div class="flex shrink-0 items-center gap-2">
								{#if !v.default}
									<Button
										variant="ghost"
										class="h-8 px-2.5 text-[13px]"
										disabled={defaulting !== null}
										onclick={() => setDefault(majorOf(v.version))}
									>
										{#if defaulting === majorOf(v.version)}<LoaderIcon class="animate-spin" />{/if}
										Make default
									</Button>
								{/if}
								<Button
									variant="ghost"
									size="icon"
									class="size-8 text-muted-foreground hover:text-destructive"
									aria-label={`Remove ${v.version}`}
									onclick={() => (removing = v)}
								>
									<Trash2Icon class="size-4" />
								</Button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}

			<div
				class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-t bg-background px-6 py-3"
			>
				<p class="flex items-start gap-2 text-[13px] text-muted-foreground">
					<InfoIcon class="mt-0.5 size-3.5 shrink-0 text-muted-foreground/80" />
					{#if s.default}
						<span>
							Default: <span class="font-medium text-foreground">Node.js {s.default}</span>, for
							projects on Auto whose repository names no version.
						</span>
					{:else}
						<span>
							No default version: projects on Auto whose repository names no version use the
							server's own node.
						</span>
					{/if}
				</p>
				{#if s.default}
					<Button
						variant="ghost"
						class="h-7 px-2 text-[13px] text-muted-foreground"
						disabled={defaulting !== null}
						onclick={() => setDefault('')}>Clear default</Button
					>
				{/if}
			</div>
		</section>

		<section class="overflow-hidden rounded-xl border bg-card" aria-labelledby="install-title">
			<div class="flex flex-col gap-5 p-6">
				<div class="flex items-start gap-4">
					<span
						class="flex size-10 shrink-0 items-center justify-center rounded-lg border border-input bg-muted"
					>
						<DownloadIcon class="size-[18px]" />
					</span>
					<div class="flex flex-col gap-1.5">
						<h2 id="install-title" class="text-xl font-semibold tracking-[-0.4px]">
							Install a version
						</h2>
						<p class="text-sm/normal text-muted-foreground">
							Downloads the official {s.platform} build, checks its SHA256 sum and keeps it on this server.
						</p>
					</div>
				</div>

				<div class="flex flex-col gap-4 sm:pl-14">
					{#if lines.length}
						<div
							class={cn(
								'grid gap-2 sm:grid-cols-2',
								lines.length === 3 ? 'lg:grid-cols-3' : 'lg:grid-cols-4'
							)}
						>
							{#each lines as r (r.version)}
								{@const major = majorOf(r.version)}
								{@const has = s.installed.some((v) => v.version === r.version)}
								{@const prog = s.installing.find((p) => majorOf(p.version) === major)}
								<button
									type="button"
									class={cn(
										'flex flex-col gap-1 rounded-lg border p-3.5 text-left transition-colors',
										has
											? 'border-input bg-background'
											: 'border-input bg-background hover:border-muted-foreground/60'
									)}
									disabled={has || installing !== null}
									onclick={() => install(major)}
								>
									<span class="flex items-center justify-between gap-2">
										<span class="text-sm font-medium">Node.js {major}</span>
										{#if has}
											<CheckIcon class="size-4 text-emerald-400" />
										{:else if prog || installing === major}
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
										{r.lts ? `LTS · ${r.lts}` : 'Current'}{has ? ' · installed' : ''}
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
							The release list from {s.mirror} could not be loaded: {releases.error.message}
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
						<Label for="node-spec" class="text-[13px] text-muted-foreground">Other version</Label>
						<div class="flex flex-col gap-2 sm:flex-row">
							<Input
								id="node-spec"
								bind:value={spec}
								placeholder="22, 20.11.1 or lts"
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
								A major version installs its newest release. Downloads come from <span
									class="font-mono">{s.mirror}</span
								>.
							</p>
						{/if}
					</form>
				</div>
			</div>
		</section>
	{/if}
</div>

<AlertDialog.Root
	open={removing !== null}
	onOpenChange={(open) => {
		if (!open && !removeBusy) removing = null;
	}}
>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Remove Node.js {removing?.version}?</AlertDialog.Title>
			<AlertDialog.Description>
				{#if removing?.used_by.length}
					{removing.used_by.join(', ')} deployed with it. A project that still asks for it installs it
					again on its next deployment.
				{:else}
					No recent deployment used it. Packages installed globally with it are removed too.
				{/if}
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
