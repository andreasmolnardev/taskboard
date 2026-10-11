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

export function parseBuildxVersion(output) {
  const match = output.match(/\bv?(\d+\.\d+\.\d+)\b/);
  return match?.[1] ?? null;
}

export function assertBuildxVersion(output, expected) {
  const actual = parseBuildxVersion(output);
  if (actual !== expected) {
    throw new Error(`Docker Buildx ${expected} is required by slopstack.yaml; found ${actual ?? 'an unknown version'}`);
  }
}

export function buildExportCommand(config, destination) {
  return {
    command: 'docker',
    args: [
      'buildx', 'build',
      '--platform', config.image.platforms.split(',').map((part) => part.trim()).join(','),
      '--tag', `${config.image.repository}:${config.image.tag}`,
      '--output', `type=oci,dest=${destination}`,
      '.',
    ],
  };
}

function display({ command, args }) {
  return [command, ...args].map((part) => (/\s/.test(part) ? JSON.stringify(part) : part)).join(' ');
}

export async function main(args = process.argv.slice(2)) {
  const exportIndex = args.indexOf('--export');
  const exportPath = exportIndex === -1 ? null : args[exportIndex + 1];
  const dryRun = args.length === 0 || args.includes('--dry-run');
  const push = args.includes('--push');
  if (args.some((arg, index) => !['--dry-run', '--push', '--export'].includes(arg) && !(exportIndex !== -1 && index === exportIndex + 1)) ||
      (exportIndex !== -1 && (!exportPath || exportPath.startsWith('--'))) ||
      (dryRun && (push || exportIndex !== -1)) || (push && exportIndex !== -1)) {
    throw new Error('Usage: pnpm deploy [--dry-run | --push | --export PATH]');
  }
  const { config } = await loadProjectConfig();
  const command = exportIndex === -1 ? buildDeployCommand(config) : buildExportCommand(config, exportPath);
  const action = push ? 'Push build' : exportIndex !== -1 ? 'OCI archive build' : 'Dry run';
  console.log(`${action}: ${display(command)}`);
  if (!push && exportIndex === -1) return 0;
  const version = spawnSync('docker', ['buildx', 'version'], { encoding: 'utf8' });
  if (version.error) throw version.error;
  if (version.status !== 0) throw new Error(`could not read Docker Buildx version: ${version.stderr || version.stdout}`);
  assertBuildxVersion(version.stdout, String(config.tools.dockerBuildx));
  const result = spawnSync(command.command, command.args, { stdio: 'inherit' });
  if (result.error) throw result.error;
  return result.status ?? 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try { process.exitCode = await main(); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
