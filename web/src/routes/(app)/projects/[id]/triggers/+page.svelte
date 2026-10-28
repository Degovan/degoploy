<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import type { RuleInput, TriggerRule } from '#lib/api/types.gen.js';
	import PageHeader from '#lib/components/page-header.svelte';
	import RuleDialog, { emptyRule, ruleToInput } from '#lib/components/rule-dialog.svelte';
	import RuleTester from '#lib/components/rule-tester.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Switch } from '#lib/components/ui/switch/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import * as AlertDialog from '#lib/components/ui/alert-dialog/index.js';
	import * as DropdownMenu from '#lib/components/ui/dropdown-menu/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { shortAgo } from '#lib/time.js';
	import { cn } from '#lib/utils.js';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import EllipsisIcon from '@lucide/svelte/icons/ellipsis';
	import GitBranchIcon from '@lucide/svelte/icons/git-branch';
	import GripVerticalIcon from '@lucide/svelte/icons/grip-vertical';
	import InfoIcon from '@lucide/svelte/icons/info';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PencilIcon from '@lucide/svelte/icons/pencil';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import TagIcon from '@lucide/svelte/icons/tag';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import ZapIcon from '@lucide/svelte/icons/zap';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();
	const canEdit = $derived(session.can('project.edit'));

	const rules = createQuery(() => ({
		queryKey: ruleKeys.list(data.id),
		queryFn: () => rulesApi.list(data.id)
	}));

	const setRules = (list: TriggerRule[]) => queryClient.setQueryData(ruleKeys.list(data.id), list);
	const refresh = () => queryClient.invalidateQueries({ queryKey: ruleKeys.list(data.id) });

	// Dialog state: `editing` null means "new rule".
	let dialogOpen = $state(false);
	let editing = $state<TriggerRule | null>(null);
	const dialogInitial = $derived(editing ? ruleToInput(editing) : emptyRule());

	function openNew() {
		editing = null;
		dialogOpen = true;
	}
	function openEdit(r: TriggerRule) {
		editing = r;
		dialogOpen = true;
	}

	async function saveRule(input: RuleInput) {
		if (editing) await rulesApi.update(data.id, editing.id, input);
		else await rulesApi.create(data.id, input);
		await refresh();
		toast.success(editing ? 'Rule saved' : 'Rule added');
	}

	async function toggle(r: TriggerRule, enabled: boolean) {
		try {
			await rulesApi.update(data.id, r.id, { ...ruleToInput(r), enabled });
		} catch (err) {
			toast.error(errorMessage(err));
		}
		await refresh();
	}

	async function saveOrder(list: TriggerRule[]) {
		try {
			setRules(
				await rulesApi.reorder(
					data.id,
					list.map((r) => r.id)
				)
			);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}

	function move(index: number, delta: -1 | 1) {
		const list = [...(rules.data ?? [])];
		const target = index + delta;
		if (target < 0 || target >= list.length) return;
		[list[index], list[target]] = [list[target]!, list[index]!];
		saveOrder(list);
	}

	// Drag to reorder: rows become draggable while their grip is held, the list is
	// rearranged live while dragging and saved on drop.
	let armed = $state<number | null>(null);
	let dragging = $state<number | null>(null);
	let preview = $state<TriggerRule[] | null>(null);
	let dropped = false;
	const list = $derived(preview ?? rules.data ?? []);

	function dragStart(e: DragEvent, r: TriggerRule) {
		if (!e.dataTransfer) return;
		dragging = r.id;
		dropped = false;
		preview = [...(rules.data ?? [])];
		e.dataTransfer.effectAllowed = 'move';
		e.dataTransfer.setData('text/plain', r.name);
	}

	function dragOver(e: DragEvent, r: TriggerRule) {
		if (dragging === null || !preview) return;
		e.preventDefault();
		if (r.id === dragging) return;
		const from = preview.findIndex((x) => x.id === dragging);
		const to = preview.findIndex((x) => x.id === r.id);
		const next = [...preview];
		next.splice(to, 0, ...next.splice(from, 1));
		preview = next;
	}

	async function dragEnd() {
		const order = preview;
		const before = rules.data ?? [];
		armed = dragging = null;
		if (dropped && order && order.some((r, i) => r.id !== before[i]?.id)) {
			await saveOrder(order);
		}
		preview = null;
	}

	function gripKey(e: KeyboardEvent, index: number) {
		if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
			e.preventDefault();
			move(index, e.key === 'ArrowUp' ? -1 : 1);
		}
	}

	// Delete confirmation, opened from a row's menu. `deleting` outlives the dialog's
	// close animation so its title doesn't go blank while fading out.
	let deleting = $state<TriggerRule | null>(null);
	let deleteOpen = $state(false);
	let deleteBusy = $state(false);
	async function remove() {
		const r = deleting;
		if (!r) return;
		deleteBusy = true;
		try {
			await rulesApi.remove(data.id, r.id);
			await refresh();
			toast.success(`Rule "${r.name}" deleted`);
			deleteOpen = false;
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			deleteBusy = false;
		}
	}

	function conditions(r: TriggerRule): [string, string][] {
		const out: [string, string][] = [[r.event === 'tag' ? 'tag' : 'branch', r.ref_pattern]];
		if (r.skip_keyword) out.push(['skip', r.skip_keyword]);
		if (r.paths_include.length) out.push(['only', r.paths_include.join(', ')]);
		if (r.paths_exclude.length) out.push(['ignore', r.paths_exclude.join(', ')]);
		const env = Object.entries(r.extra_env);
		if (env.length) out.push(['env', env.map(([k, v]) => `${k}=${v}`).join(', ')]);
		return out;
	}

	const cols =
		'grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-x-3 gap-y-3 md:grid-cols-[56px_minmax(0,1fr)_minmax(0,1fr)_48px_64px] md:gap-x-4';
</script>

<PageHeader
	title="Trigger Rules"
	description="A push deploys when it matches an enabled rule. Rules are checked top to bottom."
>
	{#snippet actions()}
		{#if canEdit}
			<Button class="h-9 px-3.5 text-[13px]" onclick={openNew}>
				<PlusIcon />
				Add Rule
			</Button>
		{/if}
	{/snippet}
</PageHeader>

<div class="container-page grid items-start gap-6 pt-8 pb-18 xl:grid-cols-[minmax(0,1fr)_22rem]">
	<section class="min-w-0 overflow-hidden rounded-xl border bg-card" aria-label="Trigger rules">
		<div
			class={cn(
				cols,
				'hidden border-b bg-background px-4 py-2.5 text-xs font-medium text-muted-foreground/80 md:grid'
			)}
			aria-hidden="true"
		>
			<span>Order</span>
			<span>Rule</span>
			<span>Conditions</span>
			<span>On</span>
			<span></span>
		</div>

		{#if rules.isPending}
			<div class="flex flex-col gap-3 p-4">
				<Skeleton class="h-12 w-full" />
				<Skeleton class="h-12 w-full" />
			</div>
		{:else if rules.isError}
			<div class="p-4">
				<Alert.Root variant="destructive">
					<CircleAlertIcon />
					<Alert.Title>{rules.error.message}</Alert.Title>
				</Alert.Root>
			</div>
		{:else if list.length === 0}
			<div class="flex flex-col items-center gap-3 px-6 py-12 text-center">
				<ZapIcon class="size-5 text-muted-foreground" />
				<p class="max-w-sm text-sm text-muted-foreground">
					No rules yet, so webhooks never deploy. Add a rule such as “push to main”.
				</p>
				{#if canEdit}
					<Button variant="outline" class="h-8 px-3 text-[13px]" onclick={openNew}>
						<PlusIcon />
						Add Rule
					</Button>
				{/if}
			</div>
		{:else}
			<ol>
				{#each list as r, i (r.id)}
					{@const last = r.last_match}
					<li
						class={cn(
							cols,
							'border-b p-4 transition-opacity last:border-b-0',
							dragging === r.id && 'opacity-40'
						)}
						draggable={armed === r.id}
						ondragstart={(e) => dragStart(e, r)}
						ondragover={(e) => dragOver(e, r)}
						ondrop={(e) => {
							e.preventDefault();
							dropped = true;
						}}
						ondragend={dragEnd}
						data-testid="rule-row"
					>
						<div class="flex items-center gap-1.5 self-start pt-0.5 md:self-center md:pt-0">
							{#if canEdit && list.length > 1}
								<button
									type="button"
									class="-ml-1 flex size-5 cursor-grab items-center justify-center rounded text-muted-foreground/80 hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none active:cursor-grabbing"
									aria-label={`Reorder ${r.name}; use arrow keys to move`}
									onpointerdown={() => (armed = r.id)}
									onpointerup={() => dragging === null && (armed = null)}
									onkeydown={(e) => gripKey(e, i)}
								>
									<GripVerticalIcon class="size-[15px]" />
								</button>
							{/if}
							<span class="font-mono text-[13px] text-muted-foreground">{i + 1}</span>
						</div>

						<div class={cn('flex min-w-0 flex-col gap-1', !r.enabled && 'opacity-55')}>
							<div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
								<span class="truncate text-sm font-medium">{r.name}</span>
								<span
									class="inline-flex h-5 shrink-0 items-center gap-1.5 rounded-full bg-muted px-2 text-[11px] text-muted-foreground"
								>
									{#if r.event === 'tag'}<TagIcon class="size-3" />Tag push{:else}<GitBranchIcon
											class="size-3"
										/>Branch push{/if}
								</span>
							</div>
							<span class="text-xs text-muted-foreground/80 md:truncate">
								{#if last}
									Last matched {shortAgo(last.at)} ·
									<a
										href={resolve('/(app)/projects/[id]/deployments/[did]', {
											id: String(data.id),
											did: String(last.deployment_id)
										})}
										class="hover:text-foreground hover:underline"
									>
										deployment #{last.deployment_id}
									</a>
								{:else}
									Never matched
								{/if}
							</span>
						</div>

						<div
							class={cn(
								'order-last col-span-full flex min-w-0 flex-col items-start gap-1.5 md:order-none md:col-span-1',
								!r.enabled && 'opacity-55'
							)}
						>
							{#each conditions(r) as [key, value] (key)}
								<span
									class="inline-flex max-w-full overflow-hidden rounded-md border border-input text-xs"
								>
									<span
										class="shrink-0 border-r border-input bg-muted px-2 py-0.5 text-muted-foreground"
										>{key}</span
									>
									<span class="truncate px-2 py-0.5 font-mono" title={value}>{value}</span>
								</span>
							{/each}
						</div>

						<Switch
							checked={r.enabled}
							disabled={!canEdit}
							onCheckedChange={(v) => toggle(r, v)}
							aria-label={`Enable ${r.name}`}
							class="data-[size=default]:h-5 data-[size=default]:w-[34px] dark:data-unchecked:border-input dark:data-unchecked:bg-muted dark:data-unchecked:[&>span]:bg-muted-foreground/80"
						/>

						<div class="flex items-center justify-end gap-0.5">
							{#if canEdit}
								<Button
									variant="ghost"
									size="icon-sm"
									class="size-[30px] text-muted-foreground [&_svg]:size-[15px]"
									onclick={() => openEdit(r)}
									aria-label={`Edit ${r.name}`}
								>
									<PencilIcon />
								</Button>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="flex size-[30px] items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none [&_svg]:size-[15px]"
										aria-label={`More actions for ${r.name}`}
									>
										<EllipsisIcon />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										<DropdownMenu.Item disabled={i === 0} onSelect={() => move(i, -1)}>
											<ArrowUpIcon />
											Move up
										</DropdownMenu.Item>
										<DropdownMenu.Item disabled={i === list.length - 1} onSelect={() => move(i, 1)}>
											<ArrowDownIcon />
											Move down
										</DropdownMenu.Item>
										<DropdownMenu.Separator />
										<DropdownMenu.Item
											variant="destructive"
											onSelect={() => {
												deleting = r;
												deleteOpen = true;
											}}
										>
											<Trash2Icon />
											Delete rule
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						</div>
					</li>
				{/each}
			</ol>
		{/if}

		<p
			class="flex items-start gap-2 border-t bg-background px-4 py-3 text-[13px] text-muted-foreground/80"
		>
			<InfoIcon class="mt-0.5 size-3.5 shrink-0" />
			<span>
				Pushes that match no rule are kept under
				<!-- eslint-disable svelte/no-navigation-without-resolve -- resolved, plus a section hash -->
				<a
					href={`${resolve('/(app)/projects/[id]', { id: String(data.id) })}#webhook-deliveries`}
					class="underline-offset-2 hover:text-foreground hover:underline">Webhook Deliveries</a
				>
				<!-- eslint-enable svelte/no-navigation-without-resolve -->
				as “ignored”.{#if canEdit && list.length > 1}<span class="hidden md:inline"
						>&nbsp;Drag rows to reorder.</span
					>{/if}
			</span>
		</p>
	</section>

	<RuleTester projectId={data.id} />
</div>

<RuleDialog
	bind:open={dialogOpen}
	initial={dialogInitial}
	mode={editing ? 'edit' : 'create'}
	onsubmit={saveRule}
/>

<AlertDialog.Root open={deleteOpen} onOpenChange={(o) => (deleteOpen = o || deleteBusy)}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete rule "{deleting?.name}"?</AlertDialog.Title>
			<AlertDialog.Description>
				Pushes that only this rule matched will no longer deploy.
			</AlertDialog.Description>
		</AlertDialog.Header>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={deleteBusy}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" onclick={remove} disabled={deleteBusy}>
				{#if deleteBusy}<LoaderIcon class="animate-spin" />{/if}
				Delete rule
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
