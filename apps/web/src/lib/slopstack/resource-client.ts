import type { ResourceDefinition } from './types';

export type ResourceQuery = {
  search?: string;
  filters?: Readonly<Record<string, string | number | boolean | readonly string[]>>;
  sort?: string;
  page?: number;
  pageSize?: number;
  signal?: AbortSignal;
};

export type ResourcePage<RecordType> = {
  items: readonly RecordType[];
  total: number;
  page?: number;
  pageSize?: number;
};

export type ResourceAdapter<RecordType, CreateType, UpdateType> = {
  list?: (query?: ResourceQuery) => Promise<ResourcePage<RecordType> | readonly RecordType[]>;
  get?: (id: string, signal?: AbortSignal) => Promise<RecordType>;
  create?: (input: CreateType) => Promise<RecordType>;
  update?: (id: string, input: UpdateType) => Promise<RecordType>;
  delete?: (id: string) => Promise<void>;
};

export type ResourceClient<RecordType extends object, CreateType extends object, UpdateType extends object> = {
  definition: ResourceDefinition<RecordType, CreateType, UpdateType>;
  list: (query?: ResourceQuery) => Promise<ResourcePage<RecordType>>;
  get: (id: string, signal?: AbortSignal) => Promise<RecordType>;
  create: (input: CreateType) => Promise<RecordType>;
  update: (id: string, input: UpdateType) => Promise<RecordType>;
  delete: (id: string) => Promise<void>;
};

function unavailable(resource: string, operation: string): never {
  throw new Error(`Resource "${resource}" does not support ${operation}.`);
}

export function createResourceClient<
  RecordType extends object,
  CreateType extends object,
  UpdateType extends object,
>(
  definition: ResourceDefinition<RecordType, CreateType, UpdateType>,
  adapter: ResourceAdapter<RecordType, CreateType, UpdateType>,
): ResourceClient<RecordType, CreateType, UpdateType> {
  const enabled = (operation: keyof NonNullable<typeof definition.operations>) => definition.operations?.[operation] !== false;

  return {
    definition,
    async list(query) {
      if (!enabled('list')) return unavailable(definition.id, 'list');
      if (!adapter.list) return unavailable(definition.id, 'list');
      const result = await adapter.list(query);
      if (Array.isArray(result)) return { items: result, total: result.length };
      return result as ResourcePage<RecordType>;
    },
    async get(id, signal) {
      if (!enabled('get')) return unavailable(definition.id, 'get');
      if (!adapter.get) return unavailable(definition.id, 'get');
      return adapter.get(id, signal);
    },
    async create(input) {
      if (!enabled('create')) return unavailable(definition.id, 'create');
      if (!adapter.create) return unavailable(definition.id, 'create');
      return adapter.create(input);
    },
    async update(id, input) {
      if (!enabled('update')) return unavailable(definition.id, 'update');
      if (!adapter.update) return unavailable(definition.id, 'update');
      return adapter.update(id, input);
    },
    async delete(id) {
      if (!enabled('delete')) return unavailable(definition.id, 'delete');
      if (!adapter.delete) return unavailable(definition.id, 'delete');
      return adapter.delete(id);
    },
  };
}
