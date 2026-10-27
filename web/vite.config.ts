import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import compression from 'compression';
import { defineConfig, type Connect, type Plugin } from 'vite';

// The Go server listens here during `make dev`; Vite proxies backend paths to it.
const backend = process.env.DEGOPLOY_BACKEND ?? 'http://127.0.0.1:8090';

// Vite serves dev modules uncompressed (~10 MB for this app, bits-ui alone is 4 MB), which
// leaves a blank page over slow remote links. Gzip them; production assets are precompressed.
function devCompression(): Plugin {
	return {
		name: 'degoploy:dev-compression',
		apply: 'serve',
		configureServer(server) {
			// @types/compression is typed against Express; at runtime it is plain Connect middleware.
			server.middlewares.use(compression() as unknown as Connect.NextHandleFunction);
		}
	};
}

export default defineConfig({
	plugins: [
		devCompression(),
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Built straight into the Go embed directory (internal/web/dist).
			adapter: adapter({
				pages: '../internal/web/dist',
				assets: '../internal/web/dist',
				fallback: 'index.html',
				// Emits .br/.gz siblings; the Go server picks one per Accept-Encoding.
				precompress: true
			}),
			csp: {
				mode: 'hash',
				directives: {
					'default-src': ['self'],
					'script-src': ['self'],
					'style-src': ['self', 'unsafe-inline'],
					'img-src': ['self', 'data:'],
					'font-src': ['self', 'data:'],
					'connect-src': ['self'],
					'object-src': ['none'],
					'base-uri': ['self'],
					'form-action': ['self']
				}
			}
		})
	],
	server: {
		// Listen on all interfaces so the dev UI is reachable from other machines.
		// The Go API stays on 127.0.0.1 and is only reached through this proxy.
		host: '0.0.0.0',
		port: 5173,
		strictPort: true,
		proxy: {
			'/api': backend,
			'/auth': backend,
			'/hook': backend,
			'/ws': { target: backend.replace(/^http/, 'ws'), ws: true }
		}
	}
});
