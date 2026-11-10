import type PocketBase from 'pocketbase';
import type { ResourceAdapter, ResourceQuery } from './resource-client';
import type { ResourceDefinition } from './types';

/** Adapts a PocketBase collection to the common resource client contract. */
export function createPocketBaseResourceAdapter<
  RecordType extends object,
  CreateType extends object,
  UpdateType extends object,
>(
  pb: PocketBase,
  definition: ResourceDefinition<RecordType, CreateType, UpdateType>,
): ResourceAdapter<RecordType, CreateType, UpdateType> {
  if (definition.source.kind !== 'pocketbase') {
    throw new Error(`Resource "${definition.id}" does not use a PocketBase source.`);
  }
  const collection = pb.collection(definition.source.collection);

  return {
    async list(query: ResourceQuery = {}) {
      const clauses: string[] = [];
      const values: Record<string, string | number | boolean> = {};
      if (query.search?.trim() && definition.query?.searchFields?.length) {
        const terms = definition.query.searchFields.map((field, index) => {
          values[`search${index}`] = query.search!.trim();
          return `${field} ~ {:search${index}}`;
        });
        clauses.push(`(${terms.join(' || ')})`);
      }
      for (const [id, raw] of Object.entries(query.filters ?? {})) {
        const filter = definition.query?.filters?.find((item) => item.id === id);
        if (!filter || raw === '' || raw === undefined) continue;
        const field = filter.field;
        if (!/^[A-Za-z][A-Za-z0-9_]*$/.test(field)) continue;
        const parameter = `filter${Object.keys(values).length}`;
        const operator = filter.operator === 'contains' ? '~'
          : filter.operator === 'startsWith' ? '~'
            : filter.operator === 'greaterThan' ? '>'
              : filter.operator === 'lessThan' ? '<'
                : filter.operator === 'in' ? '?='
                  : '=';
        if (filter.operator === 'in' && Array.isArray(raw)) {
          clauses.push(`(${raw.map((item, index) => {
            const name = `${parameter}_${index}`;
            values[name] = item;
            return `${field} ?= {:${name}}`;
          }).join(' || ')})`);
        } else {
          values[parameter] = typeof raw === 'string' || typeof raw === 'number' || typeof raw === 'boolean' ? raw : [...raw].join(',');
          const suffix = filter.operator === 'startsWith' ? '%' : '';
          clauses.push(`${field} ${operator} {:${parameter}}${suffix}`);
        }
      }
      const filter = clauses.length ? pb.filter(clauses.join(' && '), values) : undefined;
      const result = await collection.getList(query.page ?? 1, query.pageSize ?? definition.query?.pageSize ?? 25, {
        sort: query.sort ?? definition.query?.defaultSort,
        filter,
        signal: query.signal,
      });
      return { items: result.items as RecordType[], total: result.totalItems, page: result.page, pageSize: result.perPage };
    },
    async get(id, signal) { return await collection.getOne(id, { signal }) as RecordType; },
    async create(input) { return await collection.create(input) as RecordType; },
    async update(id, input) { return await collection.update(id, input) as RecordType; },
    async delete(id) { await collection.delete(id); },
  };
}
