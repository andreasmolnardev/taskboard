#!/usr/bin/env node
import { spawnSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';
import { loadProjectConfig } from './project-config.js';

export function buildDeployCommand(config) {
  return {
    command: 'docker',
    args: [
      'buildx', 'build',
      '--platform', config.image.platforms.split(',').map((part) => part.trim()).join(','),
      '--tag', `${config.image.repository}:${config.image.tag}`,
      '--push',
      '.',
    ],
  };
}

function display({ command, args }) {
  return [command, ...args].map((part) => (/\s/.test(part) ? JSON.stringify(part) : part)).join(' ');
}

export async function main(args = process.argv.slice(2)) {
  const dryRun = args.length === 0 || args.includes('--dry-run');
  const push = args.includes('--push');
  if (args.some((arg) => !['--dry-run', '--push'].includes(arg)) || (dryRun && push)) {
    throw new Error('Usage: pnpm deploy [--dry-run | --push]');
  }
  const { config } = await loadProjectConfig();
  const command = buildDeployCommand(config);
  console.log(`${push ? 'Push build' : 'Dry run'}: ${display(command)}`);
  if (!push) return 0;
  const result = spawnSync(command.command, command.args, { stdio: 'inherit' });
  if (result.error) throw result.error;
  return result.status ?? 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { process.exitCode = await main(); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
