export const taskboardQueryKeys = {
  user: (userId: string) => ['taskboard', 'user', userId] as const,
  homeData: (userId: string) => ['taskboard', 'user', userId, 'home-data'] as const,
  search: (userId: string, query: string) =>
    ['taskboard', 'user', userId, 'deep-search', query] as const,
  notifications: (userId: string) =>
    ['taskboard', 'user', userId, 'notifications'] as const,
};
