import type { FieldDefinition, ResourceDefinition } from './types';

function hasValue(value: unknown) {
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'string') return value.trim().length > 0;
  return value !== undefined && value !== null;
}

export function validateField(field: FieldDefinition, value: unknown, record: unknown): string | undefined {
  if (field.required && !hasValue(value)) return `${field.label} is required.`;
  if (!hasValue(value)) return undefined;

  if (field.kind === 'text' && typeof value === 'string') {
    if (field.minLength !== undefined && value.length < field.minLength) {
      return `${field.label} must contain at least ${field.minLength} characters.`;
    }
    if (field.maxLength !== undefined && value.length > field.maxLength) {
      return `${field.label} must contain no more than ${field.maxLength} characters.`;
    }
    if (field.pattern && !new RegExp(field.pattern).test(value)) return `${field.label} has an invalid format.`;
  }

  if ((field.kind === 'number' || field.kind === 'slider') && typeof value === 'number') {
    if (field.min !== undefined && value < field.min) return `${field.label} must be at least ${field.min}.`;
    if (field.max !== undefined && value > field.max) return `${field.label} must be no more than ${field.max}.`;
  }

  if (field.kind === 'select') {
    const selected = Array.isArray(value) ? value : [value];
    if (field.multiple && !Array.isArray(value)) return `${field.label} must contain a list of selections.`;
    if (selected.some((item) => typeof item !== 'string' || !field.options.some((option) => option.value === item))) {
      return `${field.label} has an invalid selection.`;
    }
  }

  if (field.kind === 'relation') {
    const values = Array.isArray(value) ? value : [value];
    if (field.cardinality === 'one' && values.length > 1) return `${field.label} accepts one related item.`;
    if (values.some((item) => typeof item !== 'string')) return `${field.label} contains an invalid relationship.`;
  }

  if ((field.kind === 'date' || field.kind === 'datetime' || field.kind === 'time') && typeof value === 'string') {
    if (field.min && value < field.min) return `${field.label} must be on or after ${field.min}.`;
    if (field.max && value > field.max) return `${field.label} must be on or before ${field.max}.`;
  }

  if (field.kind === 'slider' && typeof value === 'number' && field.step && (value - field.min) % field.step !== 0) {
    return `${field.label} must use increments of ${field.step}.`;
  }

  if (field.kind === 'file') {
    const files = Array.isArray(value) ? value : [value];
    if (field.multiple === false && files.length > 1) return `${field.label} accepts one file.`;
    if (field.maxFiles !== undefined && files.length > field.maxFiles) return `${field.label} accepts no more than ${field.maxFiles} file${field.maxFiles === 1 ? '' : 's'}.`;
    if (field.maxSizeBytes && typeof File !== 'undefined' && files.some((file) => file instanceof File && file.size > field.maxSizeBytes!)) {
      return `${field.label} exceeds the maximum file size.`;
    }
    if (field.accept?.length && typeof File !== 'undefined') {
      const invalid = files.some((file) => file instanceof File && !field.accept?.some((accepted) => {
        if (accepted.startsWith('.')) return file.name.toLocaleLowerCase().endsWith(accepted.toLocaleLowerCase());
        if (accepted.endsWith('/*')) return file.type.startsWith(accepted.slice(0, -1));
        return file.type === accepted;
      }));
      if (invalid) return `${field.label} contains an unsupported file type.`;
    }
  }

  const validate = field.validate as ((input: unknown, record: unknown) => string | undefined) | undefined;
  return validate?.(value, record);
}

export function validateResourceInput(
  resource: ResourceDefinition,
  values: Record<string, unknown>,
) {
  return Object.fromEntries(
    resource.fields
      .map((field) => [field.name, validateField(field, values[field.name], values)] as const)
      .filter(([, error]) => Boolean(error)),
  ) as Record<string, string>;
}
