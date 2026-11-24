import assert from 'node:assert/strict';
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { parseConfigYaml, validateProjectConfig } from './project-config.js';
import { assertBuildxVersion, buildDeployCommand, buildExportCommand } from './deploy.js';
import { planRelease } from './release.js';

const validConfig = `schemaVersion: 1
app:
  name: taskboard
  goModule: github.com/andreasmolnardev/taskboard
  envPrefix: TASKBOARD
runtime:
  port: 8090
  storagePath: /app/pb_data
development:
  webPort: 5173
  apiProxyTarget: http://127.0.0.1:8090
tools:
  go: 1.27.1
  node: 22.23.2
  pnpm: 11.8.0
  air: 1.67.4
  dockerBuildx: 0.30.0
image:
  repository: ghcr.io/andreasmolnardev/taskboard
  tag: latest
  platforms: linux/amd64,linux/arm64
registry:
  host: ghcr.io
release:
  version: 0.1.0
`;

test('validates canonical Taskboard project metadata', () => {
  const config = validateProjectConfig(parseConfigYaml(validConfig));
  assert.equal(config.app.envPrefix, 'TASKBOARD');
  assert.equal(config.runtime.storagePath, '/app/pb_data');
});

test('rejects stored credentials and duplicate YAML keys', () => {
  assert.throws(() => validateProjectConfig(parseConfigYaml(validConfig.replace('  envPrefix: TASKBOARD', '  envPrefix: TASKBOARD\n  registryToken: nope'))), /must not store credentials/);
  assert.throws(() => parseConfigYaml(validConfig.replace('  port: 8090', '  port: 8090\n  port: 8091')), /duplicate key port/);
});

test('deploy command builds with the published runtime module and configured platforms', () => {
  const config = validateProjectConfig(parseConfigYaml(validConfig));
  const command = buildDeployCommand(config);
  assert.equal(command.command, 'docker');
  assert.deepEqual(command.args, [
    'buildx', 'build', '--platform', 'linux/amd64,linux/arm64',
    '--tag', 'ghcr.io/andreasmolnardev/taskboard:latest', '--push', '.',
  ]);
});

test('deploy can export a multi-platform OCI archive and checks the configured Buildx version', () => {
  const config = validateProjectConfig(parseConfigYaml(validConfig));
  assert.deepEqual(buildExportCommand(config, '/tmp/taskboard image.tar').args, [
    'buildx', 'build', '--platform', 'linux/amd64,linux/arm64',
    '--tag', 'ghcr.io/andreasmolnardev/taskboard:latest',
    '--output', 'type=oci,dest=/tmp/taskboard image.tar', '.',
  ]);
  assert.doesNotThrow(() => assertBuildxVersion('github.com/docker/buildx v0.30.0 abcdef', '0.30.0'));
  assert.throws(() => assertBuildxVersion('github.com/docker/buildx v0.29.0 abcdef', '0.30.0'), /0\.30\.0 is required/);
});

test('release planning changes only release version and image tag without writing', async (t) => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'taskboard-release-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const configPath = path.join(directory, 'slopstack.yaml');
  await writeFile(configPath, validConfig);
  const plan = await planRelease(directory, '1.2.3');
  assert.equal(plan.targetVersion, '1.2.3');
  assert.match(plan.nextSource, /  tag: "1\.2\.3"/);
  assert.match(plan.nextSource, /  version: "1\.2\.3"/);
  assert.equal(await readFile(configPath, 'utf8'), validConfig);
});
