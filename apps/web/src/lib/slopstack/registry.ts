import type {
  ActionDefinition,
  BackgroundOperationDefinition,
  FieldRendererDefinition,
  AnyResourceDefinition,
  ApplicationDefinition,
  AuthenticationDefinition,
  LayoutDefinition,
  SearchProvider,
  SearchFallbackDefinition,
  SettingsDefinition,
  ShortcutDefinition,
  SlopstackPlugin,
  ViewDefinition,
} from './types';

type Identified = { id: string };

function addUnique<T extends Identified>(items: Map<string, T>, item: T, kind: string) {
  if (items.has(item.id)) throw new Error(`Duplicate ${kind} id "${item.id}".`);
  items.set(item.id, item);
}

export class SlopstackRegistry {
  readonly resources = new Map<string, AnyResourceDefinition>();
  readonly views = new Map<string, ViewDefinition>();
  readonly layouts = new Map<string, LayoutDefinition>();
  readonly actions = new Map<string, ActionDefinition>();
  readonly searchProviders = new Map<string, SearchProvider>();
  readonly searchFallbacks = new Map<string, SearchFallbackDefinition>();
  readonly settings = new Map<string, SettingsDefinition>();
  readonly backgroundOperations = new Map<string, BackgroundOperationDefinition>();
  readonly fieldRenderers = new Map<string, FieldRendererDefinition>();
  authentication: AuthenticationDefinition | undefined;
  readonly shortcuts = new Map<string, ShortcutDefinition>();
  readonly plugins = new Map<string, SlopstackPlugin>();

  registerResource(resource: AnyResourceDefinition) {
    addUnique(this.resources, resource, 'resource');
  }

  registerView(view: ViewDefinition) {
    addUnique(this.views, view, 'view');
  }

  registerLayout(layout: LayoutDefinition) {
    addUnique(this.layouts, layout, 'layout');
  }

  registerAction(action: ActionDefinition) {
    addUnique(this.actions, action, 'action');
  }

  registerSearchProvider(provider: SearchProvider) {
    addUnique(this.searchProviders, provider, 'search provider');
  }

  registerSearchFallback(fallback: SearchFallbackDefinition) {
    addUnique(this.searchFallbacks, fallback, 'search fallback');
  }

  registerSettings(settings: SettingsDefinition) {
    addUnique(this.settings, settings, 'settings definition');
  }

  registerBackgroundOperation(operation: BackgroundOperationDefinition) {
    addUnique(this.backgroundOperations, operation, 'background operation');
  }

  registerFieldRenderer(renderer: FieldRendererDefinition) {
    addUnique(this.fieldRenderers, renderer, 'field renderer');
  }

  registerShortcut(shortcut: ShortcutDefinition) {
    addUnique(this.shortcuts, shortcut, 'shortcut');
  }

  registerPlugin(plugin: SlopstackPlugin) {
    addUnique(this.plugins, plugin, 'plugin');
  }

  validate(application: ApplicationDefinition, includePluginContributions = false) {
    const issues: string[] = [];

    for (const view of this.views.values()) {
      if (view.layout && !this.layouts.has(view.layout)) {
        issues.push(`View "${view.id}" refers to missing layout "${view.layout}".`);
      }
    }

    const routes = new Map<string, string>();
    for (const view of this.views.values()) {
      if (!view.path.startsWith('/')) issues.push(`View "${view.id}" path must start with /.`);
      const signature = view.path.replace(/:[A-Za-z][A-Za-z0-9_]*/g, ':param').replace(/\/+$/, '') || '/';
      const existing = routes.get(signature);
      if (existing) issues.push(`Views "${existing}" and "${view.id}" have conflicting route patterns "${view.path}".`);
      else routes.set(signature, view.id);
    }

    for (const layout of this.layouts.values()) {
      const slotIds = new Set<string>();
      for (const slot of layout.slots) {
        if (slotIds.has(slot.id)) issues.push(`Layout "${layout.id}" has duplicate slot "${slot.id}".`);
        slotIds.add(slot.id);
      }
    }

    for (const resource of this.resources.values()) {
      const fields = new Set<string>();
      for (const field of resource.fields) {
        if (fields.has(field.name)) issues.push(`Resource "${resource.id}" has duplicate field "${field.name}".`);
        fields.add(field.name);
        if (field.kind === 'text' && field.pattern) {
          try {
            new RegExp(field.pattern);
          } catch {
            issues.push(`Resource "${resource.id}" field "${field.name}" has an invalid pattern.`);
          }
        }
        if (includePluginContributions && field.kind === 'custom' && !this.fieldRenderers.has(field.renderer)) {
          issues.push(`Resource "${resource.id}" field "${field.name}" refers to missing field renderer "${field.renderer}".`);
        }
      }
      for (const field of resource.query?.searchFields ?? []) {
        if (!fields.has(field)) issues.push(`Resource "${resource.id}" searches unknown field "${field}".`);
      }
      for (const filter of resource.query?.filters ?? []) {
        if (!fields.has(filter.field)) issues.push(`Resource "${resource.id}" filter "${filter.id}" uses unknown field "${filter.field}".`);
      }
      const relationships = [
        ...(resource.relations ?? []).map((relation) => relation.resource),
        ...resource.fields.filter((field) => field.kind === 'relation').map((field) => field.resource),
      ];
      for (const target of relationships) {
        if (!this.resources.has(target)) {
          issues.push(`Resource "${resource.id}" refers to missing relationship resource "${target}".`);
        }
      }
    }

    const navigationIds = new Set<string>();
    for (const item of application.shell?.navigation ?? []) {
      if (navigationIds.has(item.id)) issues.push(`Shell navigation has duplicate item "${item.id}".`);
      navigationIds.add(item.id);
      if (!this.views.has(item.view)) {
        issues.push(`Navigation item "${item.id}" refers to missing view "${item.view}".`);
      }
    }
    const regionIds = new Set<string>();
    for (const region of application.shell?.persistentRegions ?? []) {
      if (regionIds.has(region.id)) issues.push(`Shell has duplicate persistent region "${region.id}".`);
      regionIds.add(region.id);
    }

    if (issues.length) throw new Error(`Invalid Slopstack application:\n- ${issues.join('\n- ')}`);
  }
}

export function createSlopstackRegistry(application: ApplicationDefinition) {
  const registry = new SlopstackRegistry();
  registry.authentication = application.authentication;
  for (const resource of application.resources ?? []) registry.registerResource(resource);
  for (const layout of application.layouts ?? []) registry.registerLayout(layout);
  for (const view of application.views ?? []) registry.registerView(view);
  for (const action of application.actions ?? []) registry.registerAction(action);
  for (const provider of application.searchProviders ?? []) registry.registerSearchProvider(provider);
  for (const fallback of application.searchFallbacks ?? []) registry.registerSearchFallback(fallback);
  for (const settings of application.settings ?? []) registry.registerSettings(settings);
  for (const operation of application.backgroundOperations ?? []) registry.registerBackgroundOperation(operation);
  for (const renderer of application.fieldRenderers ?? []) registry.registerFieldRenderer(renderer);
  for (const shortcut of application.shortcuts ?? []) registry.registerShortcut(shortcut);
  for (const plugin of application.plugins ?? []) registry.registerPlugin(plugin);

  registry.validate(application);

  const cleanup: Array<() => void> = [];
  let disposed = false;
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    for (const disposePlugin of [...cleanup].reverse()) disposePlugin();
  };
  try {
    for (const plugin of registry.plugins.values()) {
      const dispose = plugin.register(registry);
      if (dispose) cleanup.push(dispose);
    }
    registry.validate(application, true);
  } catch (error) {
    dispose();
    throw error;
  }

  return {
    registry,
    dispose,
  };
}
