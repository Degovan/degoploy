<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { toast } from 'svelte-sonner';
	import { ApiError, errorMessage } from '#lib/api/client.js';
	import { cacheDeployment, deploymentKeys, deploymentsApi } from '#lib/api/deployments.js';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Input } from '#lib/components/ui/input/index.js';
	import { Label } from '#lib/components/ui/label/index.js';
	import * as Dialog from '#lib/components/ui/dialog/index.js';
	import * as RadioGroup from '#lib/components/ui/radio-group/index.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import RocketIcon from '@lucide/svelte/icons/rocket';

	let { projectId, size = 'default' }: { projectId: number; size?: 'default' | 'sm' } = $props();
	const queryClient = useQueryClient();

	const rules = createQuery(() => ({
		queryKey: ruleKeys.list(projectId),
		queryFn: () => rulesApi.list(projectId)
	}));

	let open = $state(false);
	let kind = $state<'branch' | 'tag'>('branch');
	let name = $state('');
	let error = $state('');
	let busy = $state(false);

	// Suggest the first concrete branch from the push rules, else "main".
	function suggestion(): string {
		for (const r of rules.data ?? []) {
			if (r.event !== 'push') continue;
			const p = r.ref_pattern.split(',')[0]?.trim();
			if (p && !/[*?]/.test(p)) return p;
		}
		return 'main';
	}

	function show() {
		kind = 'branch';
		name = suggestion();
		error = '';
		open = true;
	}

	async function deploy(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const ref = `${kind === 'tag' ? 'refs/tags/' : 'refs/heads/'}${name.trim()}`;
			const d = await deploymentsApi.deploy(projectId, ref);
			cacheDeployment(queryClient, d);
			await queryClient.invalidateQueries({ queryKey: deploymentKeys.list(projectId) });
			open = false;
			toast.success(`Deployment #${d.id} queued`);
			await goto(
				resolve('/(app)/projects/[id]/deployments/[did]', {
					id: String(projectId),
					did: String(d.id)
				})
			);
		} catch (err) {
			error = err instanceof ApiError ? err.message : errorMessage(err);
		} finally {
			busy = false;
		}
	}
</script>

<Button {size} onclick={show}>
	<RocketIcon />
	Deploy now
</Button>

<Dialog.Root bind:open>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Deploy now</Dialog.Title>
			<Dialog.Description>
				Runs the deploy script for the latest commit of a branch or tag. The matching trigger rule's
				environment is applied.
			</Dialog.Description>
		</Dialog.Header>
		<form id="deploy-form" class="flex flex-col gap-4" onsubmit={deploy}>
			<RadioGroup.Root bind:value={kind} class="flex gap-6">
				<div class="flex items-center gap-2">
					<RadioGroup.Item value="branch" id="deploy-branch" />
					<Label for="deploy-branch" class="font-normal">Branch</Label>
				</div>
				<div class="flex items-center gap-2">
					<RadioGroup.Item value="tag" id="deploy-tag" />
					<Label for="deploy-tag" class="font-normal">Tag</Label>
				</div>
			</RadioGroup.Root>
			<div class="flex flex-col gap-2">
				<Label for="deploy-ref">{kind === 'tag' ? 'Tag' : 'Branch'}</Label>
				<Input id="deploy-ref" required bind:value={name} class="font-mono" />
				{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
			</div>
		</form>
		<Dialog.Footer>
			<Button variant="ghost" onclick={() => (open = false)} disabled={busy}>Cancel</Button>
			<Button type="submit" form="deploy-form" disabled={busy || !name.trim()}>
				{#if busy}<LoaderIcon class="animate-spin" />{:else}<RocketIcon />{/if}
				Deploy
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>
