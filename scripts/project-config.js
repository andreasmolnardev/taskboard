import fs from 'node:fs/promises';
import path from 'node:path';

const secretKey = /(secret|password|token|credential|api[_-]?key)/i;

function scalar(source, lineNumber) {
  const value = source.trim();
  if (!value) throw new Error(`empty YAML value on line ${lineNumber}`);
  if (value.startsWith('"')) {
    try { return JSON.parse(value); }
    catch { throw new Error(`invalid quoted value on line ${lineNumber}`); }
  }
  if (value.startsWith("'")) {
    if (!value.endsWith("'")) throw new Error(`invalid quoted value on line ${lineNumber}`);
    return value.slice(1, -1).replaceAll("''", "'");
  }
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (/^-?(0|[1-9]\d*)$/.test(value)) return Number(value);
  return value;
}

function stripComment(line) {
  let quote = null;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (quote === '"' && char === '\\') { index += 1; continue; }
    if ((char === '"' || char === "'") && (!quote || quote === char)) {
      quote = quote ? null : char;
      continue;
    }
    if (char === '#' && !quote && (index === 0 || /\s/.test(line[index - 1]))) return line.slice(0, index).trimEnd();
  }
  return line.trimEnd();
}

export function parseConfigYaml(text) {
  const root = {};
  const stack = [{ indent: -2, value: root }];
  const lines = text.replace(/^\uFEFF/, '').split(/\r?\n/);
  lines.forEach((rawLine, index) => {
    if (/\t/.test(rawLine)) throw new Error(`tabs are not supported in slopstack.yaml on line ${index + 1}`);
    const line = stripComment(rawLine);
    if (!line.trim()) return;
    const indent = line.length - line.trimStart().length;
    if (indent % 2 !== 0) throw new Error(`indentation must use pairs of spaces on line ${index + 1}`);
    const match = line.trim().match(/^([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*))?$/);
    if (!match) throw new Error(`unsupported YAML syntax on line ${index + 1}`);
    while (stack.at(-1).indent >= indent) stack.pop();
    const parent = stack.at(-1);
    if (indent !== parent.indent + 2) throw new Error(`unexpected indentation on line ${index + 1}`);
    const [, key, rawValue = ''] = match;
    if (Object.hasOwn(parent.value, key)) throw new Error(`duplicate key ${key} on line ${index + 1}`);
    if (!rawValue.trim()) {
      parent.value[key] = {};
      stack.push({ indent, value: parent.value[key] });
    } else {
      parent.value[key] = scalar(rawValue, index + 1);
    }
  });
  return root;
}

function requireString(config, pathText) {
  const value = pathText.split('.').reduce((current, key) => current?.[key], config);
  if (typeof value !== 'string' || value.length === 0) throw new Error(`slopstack.yaml requires ${pathText} to be a non-empty string`);
  return value;
}

export function validateProjectConfig(config) {
  if (config.schemaVersion !== 1) throw new Error('slopstack.yaml schemaVersion must be 1');
  const scan = (value, location = '') => {
    if (!value || typeof value !== 'object') return;
    for (const [key, child] of Object.entries(value)) {
      const next = location ? `${location}.${key}` : key;
      if (secretKey.test(key)) throw new Error(`slopstack.yaml must not store credentials (${next})`);
      scan(child, next);
    }
  };
  scan(config);
  requireString(config, 'app.name');
  requireString(config, 'app.goModule');
  const envPrefix = requireString(config, 'app.envPrefix');
  if (!/^[A-Z][A-Z0-9_]*$/.test(envPrefix)) throw new Error('app.envPrefix must use uppercase letters, digits, and underscores');
  if (!Number.isInteger(config.runtime?.port) || config.runtime.port < 1 || config.runtime.port > 65535) {
    throw new Error('runtime.port must be an integer from 1 to 65535');
  }
  requireString(config, 'runtime.storagePath');
  if (!Number.isInteger(config.development?.webPort) || config.development.webPort < 1 || config.development.webPort > 65535) {
    throw new Error('development.webPort must be an integer from 1 to 65535');
  }
  requireString(config, 'development.apiProxyTarget');
  for (const tool of ['go', 'node', 'pnpm', 'air', 'dockerBuildx']) requireString(config, `tools.${tool}`);
  const repository = requireString(config, 'image.repository');
  if (!/^[A-Za-z0-9][A-Za-z0-9._/-]*$/.test(repository)) throw new Error('image.repository has invalid characters');
  const tag = requireString(config, 'image.tag');
  if (!/^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$/.test(tag)) throw new Error('image.tag is invalid');
  const platforms = requireString(config, 'image.platforms').split(',').map((part) => part.trim()).join(',');
  if (platforms !== 'linux/amd64,linux/arm64') throw new Error('image.platforms must be linux/amd64,linux/arm64');
  requireString(config, 'registry.host');
  if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(requireString(config, 'release.version'))) {
    throw new Error('release.version must be a semantic version');
  }
  return config;
}

export async function loadProjectConfig(projectRoot = process.cwd()) {
  const configPath = path.join(projectRoot, 'slopstack.yaml');
  const source = await fs.readFile(configPath, 'utf8');
  return { configPath, source, config: validateProjectConfig(parseConfigYaml(source)) };
}

export function setYamlScalars(source, values) {
  const wanted = new Map(Object.entries(values).map(([keyPath, value]) => [keyPath.split('.').join('/'), value]));
  const stack = [];
  const updated = new Set();
  const lines = source.split(/(\r?\n)/);
  for (let index = 0; index < lines.length; index += 2) {
    const line = lines[index];
    const indent = line.length - line.trimStart().length;
    const match = line.trimEnd().match(/^(\s*)([A-Za-z_][A-Za-z0-9_-]*):(?:\s*(.*))?$/);
    if (!match) continue;
    while (stack.length && stack.at(-1).indent >= indent) stack.pop();
    const [, , key, rawValue = ''] = match;
    const keyPath = [...stack.map((item) => item.key), key].join('/');
    if (wanted.has(keyPath)) {
      lines[index] = `${' '.repeat(indent)}${key}: ${JSON.stringify(wanted.get(keyPath))}`;
      updated.add(keyPath);
    }
    if (!rawValue.trim()) stack.push({ indent, key });
  }
  if (updated.size !== wanted.size) throw new Error('release config fields were not found in slopstack.yaml');
  return lines.join('');
}
