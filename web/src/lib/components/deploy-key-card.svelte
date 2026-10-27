<script lang="ts">
	import { toast } from 'svelte-sonner';
	import { errorMessage } from '#lib/api/client.js';
	import { projectsApi } from '#lib/api/projects.js';
	import type { ConnectionTest, Project, ProjectSecrets } from '#lib/api/types.gen.js';
	import ConfirmButton from '#lib/components/confirm-button.svelte';
	import CopyField from '#lib/components/copy-field.svelte';
	import { Button } from '#lib/components/ui/button/index.js';
	import { Skeleton } from '#lib/components/ui/skeleton/index.js';
	import { providerLabel, providerLinks } from '#lib/repo.js';
	import { cn } from '#lib/utils.js';
	import KeyRoundIcon from '@lucide/svelte/icons/key-round';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';
	import PlugIcon from '@lucide/svelte/icons/plug';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';

	let {
		project: p,
		secrets,
		onregenerate
	}: {
		project: Project;
		secrets: ProjectSecrets | undefined;
		onregenerate: () => Promise<void>;
	} = $props();

	const links = $derived(providerLinks(p.provider, p.repo_url));

	let testing = $state(false);
	let test = $state<ConnectionTest | null>(null);

	async function testConnection() {
		testing = true;
		try {
			test = await projectsApi.testConnection(p.id);
		} catch (err) {
			toast.error(errorMessage(err));
		} finally {
			testing = false;
		}
	}

	async function regenerate() {
		await onregenerate();
		test = null;
	}

	// "SHA256:MGreKIgU…/95VQCc"
	const shortFingerprint = (f: string) => {
		const [algo, hash = ''] = f.split(':');
		return hash.length > 16 ? `${algo}:${hash.slice(0, 8)}…${hash.slice(-6)}` : f;
	};
	const branchList = (bs: string[]) =>
		bs.length <= 3 ? bs.join(', ') : `${bs.slice(0, 3).join(', ')} +${bs.length - 3}`;
</script>

<div class="overflow-hidden rounded-xl border bg-card" data-testid="deploy-key-card">
	<div class="flex flex-col gap-4 p-6">
		<div class="flex flex-col gap-1.5">
			<h3 class="flex items-center gap-2 font-semibold">
				<KeyRoundIcon class="size-4" />
				Deploy Key
			</h3>
			<p class="text-sm text-muted-foreground">
				Add this public key on {providerLabel[p.provider]} under
				{#if links}
					<!-- eslint-disable svelte/no-navigation-without-resolve -- external link -->
					<a
						href={links.deployKeys}
						target="_blank"
						rel="noreferrer"
						class="underline-offset-4 hover:underline">Settings → Deploy keys</a
					>.
					<!-- eslint-enable svelte/no-navigation-without-resolve -->
				{:else}
					Settings → Deploy keys.
				{/if}
				Read-only access is enough.
			</p>
		</div>
		{#if secrets}
			<CopyField value={secrets.public_key} multiline label="public key" />
		{:else}
			<Skeleton class="h-14 w-full" />
		{/if}
		{#if test && !test.ok}
			<pre
				class="max-h-40 overflow-auto rounded-md border border-red-500/30 bg-red-500/10 p-3 font-mono text-xs whitespace-pre-wrap text-red-600 dark:text-red-400">{test.message}</pre>
		{/if}
	</div>
	<div class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t bg-background/40 px-6 py-3">
		<span class="flex min-w-0 items-center gap-2 text-sm" data-testid="connection-status">
			<span
				class={cn(
					'size-2 shrink-0 rounded-full',
					!test ? 'bg-zinc-500' : test.ok ? 'bg-emerald-400' : 'bg-red-500'
				)}
			></span>
			<span class={cn(!test && 'text-muted-foreground')}>
				{#if !test}
					Not tested
				{:else if test.ok}
					Connected
					{#if test.branches.length}
						<span class="text-muted-foreground">· {branchList(test.branches)}</span>
					{/if}
				{:else}
					Connection failed
				{/if}
			</span>
		</span>
		{#if secrets}
			<span
				class="hidden truncate font-mono text-xs text-muted-foreground sm:inline"
				title={secrets.key_fingerprint}>{shortFingerprint(secrets.key_fingerprint)}</span
			>
		{/if}
		<div class="ml-auto flex items-center gap-2">
			<ConfirmButton
				size="sm"
				variant="ghost"
				title="Generate a new deploy key?"
				description="The current key stops working immediately. Deploys fail until you add the new public key to the repository."
				confirmLabel="Generate new key"
				onconfirm={regenerate}
			>
				<RefreshCwIcon />
				Regenerate
			</ConfirmButton>
			<Button variant="outline" size="sm" onclick={testConnection} disabled={testing}>
				{#if testing}<LoaderIcon class="animate-spin" />{:else}<PlugIcon />{/if}
				Test connection
			</Button>
		</div>
	</div>
</div>
