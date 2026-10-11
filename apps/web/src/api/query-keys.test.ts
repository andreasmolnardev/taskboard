import { describe, expect, it } from 'vitest';
import { taskboardQueryKeys } from './query-keys';

describe('taskboard query keys', () => {
  it('isolates server data by authenticated user', () => {
    expect(taskboardQueryKeys.homeData('alice')).not.toEqual(
      taskboardQueryKeys.homeData('bob'),
    );
    expect(taskboardQueryKeys.notifications('alice')).not.toEqual(
      taskboardQueryKeys.notifications('bob'),
    );
  });

  it('keys search results by request query', () => {
    expect(taskboardQueryKeys.search('alice', 'calendar')).toEqual(
      taskboardQueryKeys.search('alice', 'calendar'),
    );
    expect(taskboardQueryKeys.search('alice', 'calendar')).not.toEqual(
      taskboardQueryKeys.search('alice', 'tasks'),
    );
  });
});
