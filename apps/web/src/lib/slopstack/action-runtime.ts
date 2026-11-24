import type { ActionDefinition, ActionContext, PermissionRequirement } from './types';

export type ActionRuntime<Input> = {
  navigate: ActionContext<Input>['navigate'];
  authorize: (requirement?: PermissionRequirement) => boolean | Promise<boolean>;
  confirm: (prompt: NonNullable<ActionDefinition<Input>['confirm']>) => boolean | Promise<boolean>;
  notify: ActionContext<Input>['notify'];
};

export type ActionResult<Output> =
  | { status: 'completed'; value: Output }
  | { status: 'cancelled' | 'denied' };

export async function runAction<Input, Output>(
  action: ActionDefinition<Input, Output>,
  input: Input,
  runtime: ActionRuntime<Input>,
): Promise<ActionResult<Output>> {
  const allowed = await runtime.authorize(action.access);
  if (!allowed) {
    runtime.notify({ title: 'Action unavailable', description: 'You do not have permission to perform this action.', type: 'error' });
    return { status: 'denied' };
  }

  if (action.confirm && !(await runtime.confirm(action.confirm))) return { status: 'cancelled' };

  try {
    const value = await action.execute({ input, navigate: runtime.navigate, notify: runtime.notify });
    if (action.messages?.success) runtime.notify({ title: action.messages.success, type: 'success' });
    return { status: 'completed', value };
  } catch (error) {
    const description = error instanceof Error ? error.message : 'Please try again.';
    runtime.notify({ title: action.messages?.error ?? 'Action failed', description, type: 'error' });
    throw error;
  }
}
