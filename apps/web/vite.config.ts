import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import fs from 'node:fs';
import { fileURLToPath, URL } from 'node:url';
import { parseConfigYaml, validateProjectConfig } from '../../scripts/project-config.js';

const projectConfigPath = fileURLToPath(new URL('../../slopstack.yaml', import.meta.url));
const projectConfig = validateProjectConfig(parseConfigYaml(fs.readFileSync(projectConfigPath, 'utf8')));

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: {
    port: projectConfig.development.webPort,
    proxy: { '/api': projectConfig.development.apiProxyTarget },
  },
});
