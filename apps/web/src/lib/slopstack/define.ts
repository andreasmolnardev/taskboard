import type {
  ActionDefinition,
  BackgroundOperationDefinition,
  FieldRendererDefinition,
  ApplicationDefinition,
  LayoutDefinition,
  ResourceDefinition,
  SearchProvider,
  SearchFallbackDefinition,
  SettingsDefinition,
  ShellDefinition,
  ShortcutDefinition,
  SlopstackPlugin,
  ViewDefinition,
} from './types';

export function defineApplication<const Definition extends ApplicationDefinition>(definition: Definition) {
  return definition;
}

export function defineResource<
  RecordType extends object,
  CreateType extends object = Partial<RecordType>,
  UpdateType extends object = Partial<CreateType>,
>(definition: ResourceDefinition<RecordType, CreateType, UpdateType>) {
  return definition;
}

export function defineView<const Definition extends ViewDefinition>(definition: Definition) {
  return definition;
}

export function defineLayout<const Definition extends LayoutDefinition>(definition: Definition) {
  return definition;
}

export function defineAction<Input = unknown, Output = unknown>(definition: ActionDefinition<Input, Output>) {
  return definition;
}

export function defineSearchProvider<const Definition extends SearchProvider>(definition: Definition) {
  return definition;
}

export function defineSearchFallback<const Definition extends SearchFallbackDefinition>(definition: Definition) {
  return definition;
}

export function defineShell<const Definition extends ShellDefinition>(definition: Definition) {
  return definition;
}

export function defineSettings<const Definition extends SettingsDefinition>(definition: Definition) {
  return definition;
}

export function defineBackgroundOperation<const Definition extends BackgroundOperationDefinition>(definition: Definition) {
  return definition;
}

export function defineFieldRenderer<const Definition extends FieldRendererDefinition>(definition: Definition) {
  return definition;
}

export function defineShortcut<const Definition extends ShortcutDefinition>(definition: Definition) {
  return definition;
}

export function definePlugin<const Definition extends SlopstackPlugin>(definition: Definition) {
  return definition;
}
