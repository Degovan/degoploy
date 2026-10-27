<script lang="ts">
	import { createQuery, useQueryClient } from '@tanstack/svelte-query';
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { projectKeys, projectsApi } from '#lib/api/projects.js';
	import type { ConnectionTest } from '#lib/api/types.gen.js';
	import CopyField from '#lib/components/copy-field.svelte';
	import WebhookDeliveries from '#lib/components/webhook-deliveries.svelte';
	import DeploymentsList from '#lib/components/deployments-list.svelte';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Badge } from '#lib/components/ui/badge/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import * as Card from '#lib/components/ui/card/index.js';
	import * as Alert from '#lib/components/ui/alert/index.js';
	import { session } from '#lib/stores/session.svelte.js';
	import { providerLabel, providerLinks } from '#lib/repo.js';
	import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
	import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PlugIcon from '@lucide/svelte/icons/plug';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const queryClient = useQueryClient();

	const project = createQuery(() => ({
		queryKey: projectKeys.detail(data.id),
		queryFn: () => projectsApi.get(data.id)
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
	const webhookURL = $derived(
		secrets.data
			? secrets.data.webhook_url || window.location.origin + secrets.data.webhook_path
			: ''
	);

	let testing = $state(false);
	let test = $state<ConnectionTest | null>(null);

	async function testConnection() {
		testing = true;
		test = null;
		try {
			test = await projectsApi.testConnection(data.id);
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			testing = false;
		}
	}

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
	<div class="grid gap-6 xl:grid-cols-3">
		<div class="flex flex-col gap-6 xl:col-span-2">
			<DeploymentsList projectId={data.id} />
			{#if session.can('project.secrets')}
				<Card.Root>
					<Card.Header>
						<Card.Title>Deploy key</Card.Title>
						<Card.Description>
							Add this public key as a read-only deploy key on the repository so degoploy can pull
							private code.
						</Card.Description>
						{#if links}
							<Card.Action>
								<Button
									variant="outline"
									size="sm"
									href={links.deployKeys}
									target="_blank"
									rel="noreferrer"
								>
									Open {providerLabel[p.provider]}
									<ExternalLinkIcon />
								</Button>
							</Card.Action>
						{/if}
					</Card.Header>
					<Card.Content class="flex flex-col gap-3">
						{#if secrets.isPending}
							<Skeleton class="h-16 w-full" />
						{:else if secrets.isError}
							<p class="text-sm text-destructive">{secrets.error.message}</p>
						{:else}
							<CopyField value={secrets.data.public_key} multiline label="public key" />
							<p class="font-mono text-xs text-muted-foreground">{secrets.data.key_fingerprint}</p>
						{/if}
					</Card.Content>
					<Card.Footer class="flex flex-wrap gap-2">
						<Button variant="outline" size="sm" onclick={testConnection} disabled={testing}>
							{#if testing}<LoaderIcon class="animate-spin" />{:else}<PlugIcon />{/if}
							Test connection
						</Button>
						<ConfirmButton
							size="sm"
							title="Generate a new deploy key?"
							description="The current key stops working immediately. Deploys fail until you add the new public key to the repository."
							confirmLabel="Generate new key"
							onconfirm={() => regenerate('key')}
						>
							<RefreshCwIcon />
							Regenerate key
						</ConfirmButton>
					</Card.Footer>
					{#if test}
						<Card.Content>
							<Alert.Root variant={test.ok ? 'default' : 'destructive'}>
								{#if test.ok}<CircleCheckIcon />{:else}<CircleAlertIcon />{/if}
								<Alert.Title>{test.ok ? test.message : 'Connection failed'}</Alert.Title>
								<Alert.Description>
									{#if test.ok}
										<div class="flex flex-wrap gap-1 pt-1">
											{#each test.branches.slice(0, 20) as b (b)}
												<Badge variant="secondary" class="font-mono">{b}</Badge>
											{/each}
										</div>
									{:else}
										<pre class="text-xs whitespace-pre-wrap">{test.message}</pre>
									{/if}
								</Alert.Description>
							</Alert.Root>
						</Card.Content>
					{/if}
				</Card.Root>

				<Card.Root>
					<Card.Header>
						<Card.Title>Webhook</Card.Title>
						<Card.Description>
							Create a webhook on the repository with these values. Content type
							<code class="text-xs">application/json</code>, push events only.
						</Card.Description>
						{#if links}
							<Card.Action>
								<Button
									variant="outline"
									size="sm"
									href={links.webhooks}
									target="_blank"
									rel="noreferrer"
								>
									Open {providerLabel[p.provider]}
									<ExternalLinkIcon />
								</Button>
							</Card.Action>
						{/if}
					</Card.Header>
					<Card.Content class="flex flex-col gap-4">
						{#if secrets.isPending}
							<Skeleton class="h-24 w-full" />
						{:else if secrets.data}
							<div class="flex flex-col gap-1.5">
								<span class="text-sm font-medium">Payload URL</span>
								<CopyField value={webhookURL} label="webhook URL" />
								{#if !secrets.data.webhook_url}
									<p class="text-xs text-muted-foreground">
										Built from this browser's address. Set <code>base_url</code> in the config so it always
										uses your public URL.
									</p>
								{/if}
							</div>
							<div class="flex flex-col gap-1.5">
								<span class="text-sm font-medium">Secret</span>
								<CopyField value={secrets.data.webhook_secret} secret label="webhook secret" />
							</div>
						{/if}
					</Card.Content>
					<Card.Footer>
						<ConfirmButton
							size="sm"
							title="Generate a new webhook secret?"
							description="Webhooks signed with the old secret are rejected until you paste the new one into the repository's webhook settings."
							confirmLabel="Generate new secret"
							onconfirm={() => regenerate('secret')}
						>
							<RefreshCwIcon />
							Regenerate secret
						</ConfirmButton>
					</Card.Footer>
				</Card.Root>
			{/if}

			<WebhookDeliveries projectId={data.id} />
		</div>

		<Card.Root class="h-fit">
			<Card.Header>
				<Card.Title>Details</Card.Title>
			</Card.Header>
			<Card.Content>
				<dl class="flex flex-col gap-3 text-sm">
					{#each [['Repository', p.repo_url, true], ['Working directory', p.work_dir, true], ['Script', p.script_source === 'managed' ? 'Managed by degoploy' : p.script_path, p.script_source === 'repo'], ['Update before run', p.auto_pull ? 'Yes' : 'No', false], ['Timeout', `${p.timeout_sec}s`, false], ['Slug', p.slug, true]] as const as [label, value, mono] (label)}
						<div class="flex flex-col gap-0.5">
							<dt class="text-muted-foreground">{label}</dt>
							<dd class={mono ? 'font-mono text-xs break-all' : ''}>{value}</dd>
						</div>
					{/each}
					{#if links}
						<!-- eslint-disable svelte/no-navigation-without-resolve -- external link -->
						<a
							href={links.repo}
							target="_blank"
							rel="noreferrer"
							class="flex items-center gap-1 text-sm underline-offset-4 hover:underline"
						>
							Open repository <ExternalLinkIcon class="size-3.5" />
						</a>
						<!-- eslint-enable svelte/no-navigation-without-resolve -->
					{/if}
				</dl>
			</Card.Content>
		</Card.Root>
	</div>
{/if}
