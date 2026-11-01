import type PocketBase from 'pocketbase';
import { createEndpointResourceAdapter, type EndpointAdapterOptions } from './endpoint-adapter';
import { createPocketBaseResourceAdapter } from './pocketbase-adapter';
import { createResourceClient } from './resource-client';
import type { ResourceDefinition } from './types';

/** Creates a typed client from the source declared on a resource. Pass a custom adapter to createResourceClient when the app owns the transport. */
export function createDefaultResourceClient<
  RecordType extends object,
  CreateType extends object,
  UpdateType extends object,
>(
  definition: ResourceDefinition<RecordType, CreateType, UpdateType>,
  options: { pocketbase?: PocketBase; endpoint?: EndpointAdapterOptions } = {},
) {
  if (definition.source.kind === 'pocketbase') {
    if (!options.pocketbase) throw new Error(`Resource "${definition.id}" needs a PocketBase client.`);
    return createResourceClient(definition, createPocketBaseResourceAdapter(options.pocketbase, definition));
  }
  return createResourceClient(definition, createEndpointResourceAdapter(definition, options.endpoint));
}
