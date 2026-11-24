import type { PermissionRequirement } from './types';

export type AccessContext = {
  authenticated: boolean;
  roles?: readonly string[];
  hasPermission?: (permission: string) => boolean;
};

/** Evaluates client-side visibility requirements. Backend handlers must independently enforce access. */
export function canAccess(requirement: PermissionRequirement | undefined, context: AccessContext) {
  if (!requirement) return true;
  if (requirement.authenticated && !context.authenticated) return false;
  if (requirement.roles?.length && !requirement.roles.some((role) => context.roles?.includes(role))) return false;
  if (requirement.permission && !context.hasPermission?.(requirement.permission)) return false;
  return true;
}
