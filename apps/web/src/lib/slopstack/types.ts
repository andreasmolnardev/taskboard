import type { ComponentType, ReactNode } from 'react';

export type SlopstackId = string;

export type PermissionRequirement = {
  authenticated?: boolean;
  permission?: string;
  roles?: readonly string[];
};

export type AuthStrategy = 'pocketbase' | 'bearer' | 'trusted-proxy' | 'custom';

export type AuthenticationDefinition = {
  /** Adapter selected by the application runtime. */
  provider: AuthStrategy;
  /** External adapters can supply a session/profile endpoint without requiring PB collections. */
  sessionEndpoint?: string;
  signInPath?: string;
  signOutPath?: string;
  /** Identifies where authorization is actually enforced. Browser hints are never security boundaries. */
  enforcement: 'pocketbase-rules' | 'server' | 'external';
  routes?: { signIn?: string; signOut?: string; unauthorized?: string };
};

export type BackendDefinition = {
  provider: 'pocketbase' | 'rest' | 'custom';
  apiBaseUrl?: string;
  /** Optional capability flags let a template avoid assuming a database or realtime service exists. */
  capabilities?: Partial<Record<'database' | 'realtime' | 'uploads' | 'admin', boolean>>;
};

export type AuthUser = {
  id: string;
  name?: string;
  email?: string;
  roles?: readonly string[];
  [claim: string]: unknown;
};

export type AuthSession = {
  user: AuthUser;
  accessToken?: string;
  expiresAt?: number;
};

export type AuthenticationAdapter<Credentials = unknown> = {
  getSession: () => Promise<AuthSession | null>;
  subscribe: (listener: (session: AuthSession | null) => void) => () => void;
  signIn: (credentials: Credentials) => Promise<AuthSession | null>;
  signOut: () => Promise<void>;
};

export type TextField = FieldBase<string, 'text'> & {
  inputType?: 'text' | 'email' | 'password' | 'url' | 'tel';
  multiline?: boolean;
  rows?: number;
  minLength?: number;
  maxLength?: number;
  pattern?: string;
};

export type NumberField = FieldBase<number, 'number'> & {
  min?: number;
  max?: number;
  step?: number;
};

export type BooleanField = FieldBase<boolean, 'boolean'>;

export type SelectField = FieldBase<string, 'select'> & {
  options: readonly { label: string; value: string; disabled?: boolean }[];
  multiple?: boolean;
};

export type DateField = FieldBase<string, 'date' | 'datetime' | 'time'> & {
  min?: string;
  max?: string;
};

export type SliderField = FieldBase<number, 'slider'> & {
  min: number;
  max: number;
  step?: number;
};

export type RelationField = FieldBase<string | readonly string[], 'relation'> & {
  resource: SlopstackId;
  cardinality: 'one' | 'many';
  displayField?: string;
};

export type FileField = FieldBase<File | string | readonly (File | string)[], 'file'> & {
  multiple?: boolean;
  maxSizeBytes?: number;
  maxFiles?: number;
  accept?: readonly string[];
};

export type CustomField = FieldBase<unknown, 'custom'> & {
  renderer: SlopstackId;
  config?: Readonly<Record<string, unknown>>;
};

export type FieldDefinition =
  | TextField
  | NumberField
  | BooleanField
  | SelectField
  | DateField
  | SliderField
  | RelationField
  | FileField
  | CustomField;

export type FieldRendererProps = {
  field: CustomField;
  value: unknown;
  error?: string;
  disabled: boolean;
  onChange: (value: unknown) => void;
};

export type FieldRendererDefinition = {
  id: SlopstackId;
  label: string;
  component: ComponentType<FieldRendererProps>;
};

export type FieldBase<Value, Kind extends string> = {
  name: string;
  label: string;
  kind: Kind;
  description?: string;
  required?: boolean;
  readOnly?: boolean;
  hidden?: boolean;
  defaultValue?: Value;
  validate?: (value: Value, record: unknown) => string | undefined;
};

export type ResourceSource =
  | { kind: 'pocketbase'; collection: string }
  | {
      kind: 'endpoint';
      list?: string;
      get?: string;
      create?: string;
      update?: string;
      delete?: string;
    };

export type QueryDefinition = {
  searchFields?: readonly string[];
  defaultSort?: string;
  sortOptions?: readonly { label: string; value: string }[];
  pageSize?: number;
  filters?: readonly FilterDefinition[];
};

export type SettingsDefinition<Value extends Record<string, unknown> = Record<string, unknown>> = {
  id: SlopstackId;
  label: string;
  description?: string;
  scope: 'user' | 'application';
  fields: readonly FieldDefinition[];
  defaults: Value;
};

export type BackgroundOperationDefinition<Input = unknown, Output = unknown> = {
  id: SlopstackId;
  label: string;
  run: (input: Input, context: {
    signal: AbortSignal;
    reportProgress: (progress: number, message?: string) => void;
  }) => Promise<Output>;
};

export type FilterDefinition = {
  id: string;
  label: string;
  field: string;
  operator: 'equals' | 'contains' | 'startsWith' | 'greaterThan' | 'lessThan' | 'in';
  options?: readonly { label: string; value: string }[];
};

export type ResourceDefinition<
  RecordType extends object = Record<string, unknown>,
  CreateType extends object = Partial<RecordType>,
  UpdateType extends object = Partial<CreateType>,
> = {
  id: SlopstackId;
  label: string;
  labelPlural?: string;
  description?: string;
  source: ResourceSource;
  fields: readonly FieldDefinition[];
  relations?: readonly { field: string; resource: SlopstackId; cardinality: 'one' | 'many' }[];
  query?: QueryDefinition;
  permissions?: Partial<Record<'list' | 'get' | 'create' | 'update' | 'delete', PermissionRequirement>>;
  operations?: Partial<Record<'list' | 'get' | 'create' | 'update' | 'delete', boolean>>;
  identify?: (record: RecordType) => string;
  types?: { record?: RecordType; create?: CreateType; update?: UpdateType };
};

/** Erases record generics where only a resource's declarative schema is needed. */
export type AnyResourceDefinition = Omit<ResourceDefinition, 'identify' | 'types'>;

export type ViewContext = {
  params: Record<string, string>;
  search: URLSearchParams;
  navigate: (to: string, options?: { replace?: boolean }) => void;
};

export type ViewDefinition = {
  id: SlopstackId;
  path: string;
  label: string;
  description?: string;
  layout?: SlopstackId;
  layoutContents?: Readonly<Record<string, LayoutContent | readonly LayoutContent[]>>;
  access?: PermissionRequirement;
  component: ComponentType<{ context: ViewContext }>;
};

export type LayoutContent = {
  kind: 'view' | 'component' | 'action' | 'resource';
  content: ReactNode;
};

export type LayoutSlot = {
  id: string;
  label?: string;
  accepts?: readonly ('view' | 'component' | 'action' | 'resource')[];
  multiple?: boolean;
  required?: boolean;
};

export type LayoutDefinition = {
  id: SlopstackId;
  label: string;
  description?: string;
  slots: readonly LayoutSlot[];
  component: ComponentType<{ slots: Record<string, ReactNode> }>;
};

export type NavigationItem = {
  id: string;
  label: string;
  view: SlopstackId;
  group?: string;
  icon?: ComponentType<{ className?: string }>;
  access?: PermissionRequirement;
};

export type ShellDefinition = {
  id: SlopstackId;
  label: string;
  navigation: readonly NavigationItem[];
  navigationStyles?: readonly ('sidebar' | 'topbar' | 'rail' | 'bottom')[];
  persistentRegions?: readonly PersistentRegionDefinition[];
  component: ComponentType<{ children: ReactNode }>;
};

export type PersistentRegionDefinition = {
  id: SlopstackId;
  label: string;
  component: ComponentType;
};

export type ActionContext<Input = unknown> = {
  input: Input;
  navigate: ViewContext['navigate'];
  notify: (notification: { title: string; description?: string; type?: 'success' | 'error' | 'info' }) => void;
};

export type ActionDefinition<Input = unknown, Output = unknown> = {
  id: SlopstackId;
  label: string;
  description?: string;
  keywords?: readonly string[];
  shortcut?: string;
  icon?: ComponentType<{ className?: string }>;
  access?: PermissionRequirement;
  confirm?: { title: string; description?: string; confirmLabel?: string; destructive?: boolean };
  execute: (context: ActionContext<Input>) => Promise<Output> | Output;
  messages?: { success?: string; error?: string };
};

export type SearchResult = {
  id: string;
  title: string;
  description?: string;
  href?: string;
  icon?: ComponentType<{ className?: string }>;
  keywords?: readonly string[];
};

export type SearchProvider = {
  id: SlopstackId;
  label: string;
  search: (query: string, signal?: AbortSignal) => Promise<readonly SearchResult[]>;
  minQueryLength?: number;
};

export type SearchFallbackDefinition = ActionDefinition<string> & {
  minQueryLength?: number;
  whenNoResults?: boolean;
  matches?: (query: string) => boolean;
};

export type ShortcutDefinition = {
  id: SlopstackId;
  keys: readonly string[];
  label: string;
  description?: string;
  access?: PermissionRequirement;
  execute: () => void | Promise<void>;
};

export type PluginContext = {
  registerResource: (resource: AnyResourceDefinition) => void;
  registerView: (view: ViewDefinition) => void;
  registerLayout: (layout: LayoutDefinition) => void;
  registerAction: (action: ActionDefinition) => void;
  registerSearchProvider: (provider: SearchProvider) => void;
  registerSearchFallback: (fallback: SearchFallbackDefinition) => void;
  registerSettings: (settings: SettingsDefinition) => void;
  registerBackgroundOperation: (operation: BackgroundOperationDefinition) => void;
  registerFieldRenderer: (renderer: FieldRendererDefinition) => void;
  registerShortcut: (shortcut: ShortcutDefinition) => void;
};

export type SlopstackPlugin = {
  id: SlopstackId;
  version: string;
  label: string;
  description?: string;
  register: (context: PluginContext) => void | (() => void);
};

export type ApplicationDefinition = {
  id: SlopstackId;
  name: string;
  backend?: BackendDefinition;
  authentication?: AuthenticationDefinition;
  shell?: ShellDefinition;
  resources?: readonly AnyResourceDefinition[];
  views?: readonly ViewDefinition[];
  layouts?: readonly LayoutDefinition[];
  actions?: readonly ActionDefinition[];
  searchProviders?: readonly SearchProvider[];
  searchFallbacks?: readonly SearchFallbackDefinition[];
  settings?: readonly SettingsDefinition[];
  backgroundOperations?: readonly BackgroundOperationDefinition[];
  fieldRenderers?: readonly FieldRendererDefinition[];
  shortcuts?: readonly ShortcutDefinition[];
  plugins?: readonly SlopstackPlugin[];
};
