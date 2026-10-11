#!/usr/bin/env node
import fs from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { loadProjectConfig, setYamlScalars } from './project-config.js';

export async function planRelease(projectRoot, version) {
  const { configPath, source, config } = await loadProjectConfig(projectRoot);
  const targetVersion = version ?? config.release.version;
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(targetVersion)) {
    throw new Error('release version must be a semantic version');
  }
  const nextSource = setYamlScalars(source, {
    'release.version': targetVersion,
    'image.tag': targetVersion,
  });
  return { configPath, currentVersion: config.release.version, targetVersion, source, nextSource };
}

export async function applyReleasePlan(plan) {
  const current = await fs.readFile(plan.configPath, 'utf8');
  if (current !== plan.source) throw new Error('slopstack.yaml changed after the release plan was created; retry');
  await fs.writeFile(plan.configPath, plan.nextSource);
}

export async function main(args = process.argv.slice(2)) {
  let version;
  let apply = false;
  for (let index = 0; index < args.length; index += 1) {
    if (args[index] === '--apply') apply = true;
    else if (args[index] === '--version' && args[index + 1]) version = args[++index];
    else if (args[index] !== '--dry-run') throw new Error('Usage: pnpm release [--dry-run] [--version x.y.z] [--apply]');
  }
  if (args.includes('--dry-run') && apply) throw new Error('choose either --dry-run or --apply');
  if (apply && !version) throw new Error('--apply requires an explicit --version');
  const plan = await planRelease(process.cwd(), version);
  console.log(`Release preparation: ${plan.currentVersion} → ${plan.targetVersion}`);
  if (!apply) {
    console.log('Dry run: slopstack.yaml would set release.version and image.tag to the target version.');
    return 0;
  }
  await applyReleasePlan(plan);
  console.log('Updated slopstack.yaml. No Git tag, image, or release was published.');
  return 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try { process.exitCode = await main(); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
