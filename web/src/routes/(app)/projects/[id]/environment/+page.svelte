<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { variableKeys, variablesApi } from '#lib/api/variables.js';
	import type { Variable, VariableInput } from '#lib/api/types.gen.js';
	import { copyText } from '#lib/clipboard.js';
	import BuiltinVariables from '#lib/components/builtin-variables.svelte';
	import PageHeader from '#lib/components/page-header.svelte';
	import VariableDialog from '#lib/components/variable-dialog.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { formatDateTime, shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CopyIcon from '@lucide/svelte/icons/copy';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import InfoIcon from '@lucide/svelte/icons/info';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import LockIcon from '@lucide/svelte/icons/lock';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();
	const canManage = $derived(session.can('project.secrets'));

	const variables = createQuery(() => ({
		queryKey: variableKeys.list(data.id),
		queryFn: () => variablesApi.list(data.id)
	}));
	const refresh = () => queryClient.invalidateQueries({ queryKey: variableKeys.list(data.id) });

	let dialogOpen = $state(false);
	let editing = $state<Variable | null>(null);

	function openNew() {
		editing = null;
		dialogOpen = true;
	}
	function openEdit(v: Variable) {
		editing = v;
		dialogOpen = true;
	}

	async function save(input: VariableInput) {
		if (editing) await variablesApi.update(data.id, editing.id, input);
		else await variablesApi.create(data.id, input);
		await refresh();
		toast.success(editing ? `${input.name} saved` : `${input.name} added`);
	}

	// Delete confirmation; `deleting` outlives the dialog's close animation.
	let deleting = $state<Variable | null>(null);
	let deleteOpen = $state(false);
	let deleteBusy = $state(false);
	async function remove() {
		const v = deleting;
		if (!v) return;
		deleteBusy = true;
		try {
			await variablesApi.remove(data.id, v.id);
			await refresh();
			toast.success(`${v.name} deleted`);
			deleteOpen = false;
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			deleteBusy = false;
		}
	}

	async function copy(text: string) {
		if (await copyText(text)) toast.success(`Copied ${text}`);
		else toast.error('Could not copy to the clipboard');
	}

	const cols =
		'grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_7.5rem_64px]';
</script>

<PageHeader
	title="Environment"
	description="Variables and secrets passed to the deploy script on every run."
>
	{#snippet actions()}
		{#if canManage}
			<Button class="h-9 px-3.5 text-[13px]" onclick={openNew}>
				<PlusIcon />
				Add Variable
			</Button>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page grid items-start gap-6 pt-8 pb-18 xl:grid-cols-[minmax(0,1fr)_20rem]">
	<section class="min-w-0 overflow-hidden rounded-xl border bg-card" aria-label="Project variables">
		<div
			class={cn(
				cols,
				'hidden border-b bg-background px-4 py-2.5 text-xs font-medium text-muted-foreground/80 md:grid'
			)}
			aria-hidden="true"
		>
			<span>Name</span>
			<span>Value</span>
			<span>Updated</span>
			<span></span>
		</div>

		{#if variables.isPending}
			<div class="flex flex-col gap-3 p-4">
				<Skeleton class="h-10 w-full" />
				<Skeleton class="h-10 w-full" />
			</div>
		{:else if variables.isError}
			<div class="p-4">
				<Alert.Root variant="destructive">
					<CircleAlertIcon />
					<Alert.Title>{variables.error.message}</Alert.Title>
				</Alert.Root>
			</div>
		{:else if variables.data.length === 0}
			<div class="flex flex-col items-center gap-3 px-6 py-12 text-center">
				<KeyRoundIcon class="size-5 text-muted-foreground" />
				<p class="max-w-sm text-sm text-muted-foreground">
					No variables yet. Add settings such as <span class="font-mono text-foreground"
						>APP_URL</span
					>
					or secrets such as <span class="font-mono text-foreground">DB_PASSWORD</span>.
				</p>
				{#if canManage}
					<Button variant="outline" class="h-8 px-3 text-[13px]" onclick={openNew}>
						<PlusIcon />
						Add Variable
					</Button>
				{/if}
			</div>
		{:else}
			<ul>
				{#each variables.data as v (v.id)}
					<li class={cn(cols, 'border-b px-4 py-3 last:border-b-0')} data-testid="variable-row">
						<div class="flex min-w-0 items-center gap-2">
							<span class="truncate font-mono text-[13px] font-medium" title={v.name}>{v.name}</span
							>
							{#if v.secret}
								<span
									class="inline-flex h-5 shrink-0 items-center gap-1 rounded-full bg-muted px-2 text-[11px] text-muted-foreground"
								>
									<LockIcon class="size-3" />Secret
								</span>
							{/if}
						</div>

						<div class="col-span-full row-start-2 min-w-0 md:col-span-1 md:row-start-auto">
							{#if v.secret}
								<span
									class="font-mono text-[13px] tracking-widest text-muted-foreground/80"
									aria-label="Hidden secret value">••••••••</span
								>
							{:else if v.value === ''}
								<span class="text-[13px] text-muted-foreground/80 italic">empty</span>
							{:else}
								<span
									class="block truncate font-mono text-[13px] text-muted-foreground"
									title={v.value}>{v.value}</span
								>
							{/if}
						</div>

						<span
							class="col-span-full row-start-3 truncate text-xs text-muted-foreground/80 md:col-span-1 md:row-start-auto"
							title={formatDateTime(v.updated_at)}
						>
							{shortAgo(v.updated_at)}{#if v.updated_by}&nbsp;· {v.updated_by}{/if}
						</span>

						<div
							class="col-start-2 row-start-1 flex items-center justify-end gap-0.5 md:col-start-auto md:row-start-auto"
						>
							{#if canManage}
								<Button
									variant="ghost"
									size="icon-sm"
									class="size-[30px] text-muted-foreground [&_svg]:size-[15px]"
									onclick={() => openEdit(v)}
									aria-label={`Edit ${v.name}`}
								>
									<PencilIcon />
								</Button>
							{/if}
							<DropdownMenu.Root>
								<DropdownMenu.Trigger
									class="flex size-[30px] items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-[15px]"
									aria-label={`More actions for ${v.name}`}
								>
									<EllipsisIcon />
								</DropdownMenu.Trigger>
								<DropdownMenu.Content align="end" class="w-48">
									<DropdownMenu.Item onSelect={() => copy(v.name)}>
										<CopyIcon />
										Copy name
									</DropdownMenu.Item>
									<DropdownMenu.Item onSelect={() => copy(`\${${v.name}}`)}>
										<CopyIcon />
										Copy reference
									</DropdownMenu.Item>
									{#if canManage}
										<DropdownMenu.Separator />
										<DropdownMenu.Item
											variant="destructive"
											onSelect={() => {
												deleting = v;
												deleteOpen = true;
											}}
										>
											<Trash2Icon />
											Delete
										</DropdownMenu.Item>
									{/if}
								</DropdownMenu.Content>
							</DropdownMenu.Root>
						</div>
					</li>
				{/each}
			</ul>
		{/if}

		<p
			class="flex items-start gap-2 border-t bg-background px-4 py-3 text-[13px] text-muted-foreground/80"
		>
			<InfoIcon class="mt-0.5 size-3.5 shrink-0" />
			<span>
				A rule's extra environment on
				<a
					href={resolve('/(app)/projects/[id]/triggers', { id: String(data.id) })}
					class="underline-offset-2 hover:text-foreground hover:underline">Triggers</a
				>
				overrides these for pushes it matches. Built-in DEPLOY_* variables always win.
			</span>
		</p>
	</section>

	<div class="flex min-w-0 flex-col gap-5">
		<section class="flex flex-col gap-3 rounded-xl border bg-card p-6" aria-label="About secrets">
			<h2 class="flex items-center gap-2 text-sm font-semibold">
				<LockIcon class="size-4 text-muted-foreground" />
				Secrets
			</h2>
			<ul class="flex list-disc flex-col gap-1.5 pl-4 text-xs/normal text-muted-foreground">
				<li>Encrypted with the server's secret key before they are stored.</li>
				<li>Write-only: after saving, a value can be replaced but not shown.</li>
				<li>Masked as <span class="font-mono">***</span> wherever the script prints them.</li>
			</ul>
		</section>
		<BuiltinVariables title="Built-in" description="Set by degoploy on every run; read-only." />
	</div>
</div>

<VariableDialog bind:open={dialogOpen} {editing} onsubmit={save} />

<AlertDialog.Root open={deleteOpen} onOpenChange={(o) => (deleteOpen = o || deleteBusy)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete {deleting?.name}?</AlertDialog.Title>
			<AlertDialog.Description>
				Deploy scripts that read it will get an empty value from the next run on.
				{#if deleting?.secret}The secret value cannot be recovered.{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={deleteBusy}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" onclick={remove} disabled={deleteBusy}>
				{#if deleteBusy}<LoaderIcon class="animate-spin" />{/if}
				Delete
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
