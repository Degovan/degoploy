<script lang="ts">
	import { onMount } from 'svelte';
	import { Terminal } from '@xterm/xterm';
	import { FitAddon } from '@xterm/addon-fit';
	import '@xterm/xterm/css/xterm.css';
	import { decodeChunk, realtime } from '#lib/realtime.svelte.js';
	import LoaderIcon from '@lucide/svelte/icons/loader-circle';

	let { deploymentId, active }: { deploymentId: number; active: boolean } = $props();

	let host: HTMLDivElement;
	let received = $state(0);
	let ended = $state(false);
	let error = $state('');

	onMount(() => {
		// Terminals read best dark regardless of the app theme.
		const term = new Terminal({
			convertEol: true,
			disableStdin: true,
			cursorStyle: 'bar',
			cursorInactiveStyle: 'none',
			fontSize: 12.5,
			lineHeight: 1.25,
			fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
			scrollback: 100_000,
			theme: {
				background: '#0c0c10',
				foreground: '#e4e4e7',
				cursor: '#0c0c10',
				selectionBackground: '#3f3f46'
			}
		});
		const fit = new FitAddon();
		term.loadAddon(fit);
		term.open(host);
		fit.fit();
		const resize = new ResizeObserver(() => fit.fit());
		resize.observe(host);

		const topic = `deployment:${deploymentId}:log`;
		const unsubscribe = realtime.subscribe(
			topic,
			(m) => {
				if (m.type === 'log.chunk') {
					const bytes = decodeChunk(m.data);
					const start = m.offset ?? 0;
					if (start > received) {
						// Missed bytes (e.g. dropped connection): ask again from what we have.
						realtime.resubscribe(topic);
						return;
					}
					const skip = received - start;
					if (skip >= bytes.length) return;
					term.write(bytes.subarray(skip));
					received = start + bytes.length;
				} else if (m.type === 'log.end') {
					ended = true;
				} else if (m.type === 'error') {
					error = String(m.data ?? 'Log unavailable');
				}
			},
			{ offset: () => received }
		);

		return () => {
			unsubscribe();
			resize.disconnect();
			term.dispose();
		};
	});
</script>

<div class="relative w-full min-w-0 overflow-hidden rounded-lg border bg-[#0c0c10]">
	<div bind:this={host} class="h-[min(70svh,40rem)] p-3" data-testid="log-viewer"></div>
	{#if received === 0}
		<div
			class="pointer-events-none absolute inset-0 flex items-center justify-center gap-2 text-sm text-zinc-400"
		>
			{#if error}
				{error}
			{:else if active && !ended}
				<LoaderIcon class="size-4 animate-spin" />
				Waiting for output…
			{:else if ended}
				No output.
			{/if}
		</div>
	{/if}
</div>
