<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { deploymentKeys, deploymentsApi } from '#lib/api/deployments.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import { ruleKeys, rulesApi } from '#lib/api/rules.js';
	import DeployKeyCard from '#lib/components/deploy-key-card.svelte';
	import LatestDeployment from '#lib/components/latest-deployment.svelte';
	import ProjectSummaryCard from '#lib/components/project-summary-card.svelte';
	import SectionHeading from '#lib/components/section-heading.svelte';
	import StageView from '#lib/components/stage-view.svelte';
	import WebhookCard from '#lib/components/webhook-card.svelte';
	import WebhookDeliveries from '#lib/components/webhook-deliveries.svelte';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { providerLabel, providerLinks } from '#lib/repo.js';
	import { session } from '#lib/stores/session.svelte.js';
	import RocketIcon from '@lucide/svelte/icons/rocket';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
	}));
	const deployments = createQuery(() => ({
		queryKey: deploymentKeys.list(data.id),
		queryFn: () => deploymentsApi.list(data.id)
	}));
	const rules = createQuery(() => ({
		queryKey: ruleKeys.list(data.id),
		queryFn: () => rulesApi.list(data.id)
	}));
	const secrets = createQuery(() => ({
		queryKey: projectKeys.secrets(data.id),
		queryFn: () => projectsApi.secrets(data.id),
		enabled: session.can('project.secrets'),
		staleTime: Infinity
	}));

	const links = $derived(
		project.data ? providerLinks(project.data.provider, project.data.repo_url) : null
	);

	async function regenerate(kind: 'key' | 'secret') {
		try {
			const next =
				kind === 'key'
					? await projectsApi.regenerateKey(data.id)
					: await projectsApi.regenerateSecret(data.id);
			queryClient.setQueryData(projectKeys.secrets(data.id), next);
			toast.success(
				kind === 'key'
					? 'New deploy key generated. Replace the old key on your Git host.'
					: 'New webhook secret generated. Update it in your webhook settings.'
			);
		} catch (err) {
			toast.error(errorMessage(err));
		}
	}
</script>

{#if project.data}
	{@const p = project.data}
	{@const list = deployments.data ?? []}

	{#if deployments.isPending}
		<Skeleton class="h-64 w-full rounded-xl" />
	{:else if deployments.isError}
		<p class="text-sm text-destructive">{deployments.error.message}</p>
	{:else if list.length === 0}
		<section class="flex flex-col gap-4">
			<SectionHeading title="Latest Deployment" />
			<div
				class="flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-12 text-center"
			>
				<RocketIcon class="mb-1 size-5 text-muted-foreground" />
				<p class="font-medium">No deployments yet</p>
				<p class="max-w-md text-sm text-muted-foreground">
					Push to a branch matched by a trigger rule, or use Deploy now to run the script right
					away.
				</p>
			</div>
		</section>
	{:else}
		<LatestDeployment project={p} deployment={list[0]!} />
		<StageView project={p} deployments={list.slice(0, 10)} />
	{/if}

	<section class="flex flex-col gap-4" aria-labelledby="repository-connection">
		<SectionHeading
			title="Repository Connection"
			description={session.can('project.secrets')
				? `Add these to ${providerLabel[p.provider]} so degoploy can pull private code and receive pushes.`
				: 'How this project is set up.'}
			id="repository-connection"
		/>
		<div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
			{#if session.can('project.secrets')}
				<div class="flex min-w-0 flex-col gap-6">
					{#if secrets.isError}
						<p class="text-sm text-destructive">{secrets.error.message}</p>
					{/if}
					<DeployKeyCard
						project={p}
						secrets={secrets.data}
						onregenerate={() => regenerate('key')}
					/>
					<WebhookCard
						project={p}
						secrets={secrets.data}
						onregenerate={() => regenerate('secret')}
					/>
				</div>
			{/if}
			<ProjectSummaryCard project={p} deployments={list} ruleCount={rules.data?.length ?? null} />
		</div>
	</section>

	<WebhookDeliveries projectId={data.id} settingsUrl={links?.webhooks} />
{/if}
