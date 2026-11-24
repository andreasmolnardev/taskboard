import { fileURLToPath } from 'node:url';
import path from 'node:path';
import fs from 'node:fs/promises';
import { loadProjectConfig } from './project-config.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const { config } = await loadProjectConfig(root);
const goMod = await fs.readFile(path.join(root, 'go.mod'), 'utf8');
const webPackage = JSON.parse(await fs.readFile(path.join(root, 'apps/web/package.json'), 'utf8'));
const dockerfile = await fs.readFile(path.join(root, 'Dockerfile'), 'utf8');

if (config.app.name !== 'taskboard') throw new Error('app.name must be taskboard');
if (config.app.goModule !== 'github.com/andreasmolnardev/taskboard') throw new Error('app.goModule must match Taskboard identity');
if (config.app.envPrefix !== 'TASKBOARD') throw new Error('app.envPrefix must be TASKBOARD');
if (!goMod.startsWith(`module ${config.app.goModule}\n`)) throw new Error('app.goModule does not match go.mod');
if (webPackage.name !== '@taskboard/web') throw new Error('Taskboard frontend package name changed unexpectedly');
for (const image of [`node:${config.tools.node}-alpine3.24 AS web-build`, `golang:${config.tools.go}-alpine3.24 AS server-build`, 'alpine:3.24.2']) {
  const escapedImage = image.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const fromLine = new RegExp(`^FROM (?:--platform=\\$BUILDPLATFORM )?${escapedImage}$`, 'm');
  if (!fromLine.test(dockerfile)) throw new Error(`Dockerfile is missing pinned base image FROM ${image}`);
}

console.log(`Taskboard metadata is valid: ${config.app.goModule}, ${config.image.repository}:${config.image.tag}`);
