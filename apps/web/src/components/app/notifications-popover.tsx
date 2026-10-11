import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Bell, X } from 'lucide-react';
import { apiFetch, apiJson } from '../../api/client';
import { pb } from '../../api/pocketbase';
import { taskboardQueryKeys } from '../../api/query-keys';

type Notification = {
  id: string;
  title: string;
  body?: string;
  read: boolean;
  created: string;
  dismissedAt?: string;
};

export function NotificationsPopover({
  open,
  closing,
  onClose,
  onAnimationEnd,
  onCountChange,
}: {
  open: boolean;
  closing: boolean;
  onClose: () => void;
  onAnimationEnd: (event: React.AnimationEvent<HTMLDivElement>) => void;
  onCountChange?: (count: number) => void;
}) {
  const queryClient = useQueryClient();
  const userId = pb.authStore.record?.id ?? '';
  const queryKey = taskboardQueryKeys.notifications(userId);
  const notificationQuery = useQuery({
    queryKey,
    enabled: Boolean(userId && (open || closing)),
    queryFn: () => apiJson<Notification[]>('/api/notifications'),
  });
  const mutation = useMutation({
    mutationFn: async ({ url, method, body }: { url: string; method: string; body?: object }) => {
      const response = await apiFetch(url, {
        method,
        ...(body ? { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) } : {}),
      });
      if (!response.ok) throw new Error('Could not update notifications.');
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey }),
  });
  const notifications = notificationQuery.data ?? [];
  const error =
    notificationQuery.error instanceof Error
      ? notificationQuery.error.message
      : mutation.error instanceof Error
        ? mutation.error.message
        : '';

  useEffect(() => {
    onCountChange?.(notifications.filter((item) => !item.read).length);
  }, [notifications, onCountChange]);

  const action = async (id: string, path: string, body?: object) => {
    mutation.mutate({ url: `/api/notifications/${id}/${path}`, method: path === 'read' ? 'PATCH' : 'POST', body });
  };
  const markAllRead = async () => {
    mutation.mutate({ url: '/api/notifications/read-all', method: 'POST' });
  };
  if (!open && !closing) return null;
  return (
    <div
      className={`notification-popover ${closing ? 'is-closing' : ''}`}
      onAnimationEnd={onAnimationEnd}
    >
      <div className="notification-popover-header">
        <h2>Notifications</h2>
        <button className="icon-button" onClick={onClose} aria-label="Close notifications">
          <X size={17} />
        </button>
      </div>
      {notifications.length === 0 ? (
        <div className="empty-state notification-empty">
          <Bell size={22} />
        <strong>{notificationQuery.isLoading ? 'Loading notifications…' : error || 'No notifications'}</strong>
        <span>{error ? 'Try again later.' : 'Updates will appear here.'}</span>
        </div>
      ) : (
        <div className="notification-list">
          {notifications.map((notification) => (
            <article key={notification.id} className={notification.read ? 'is-read' : ''}>
              <strong>{notification.title}</strong>
              <span>{notification.body}</span>
              <small>{new Date(notification.created).toLocaleString()}</small>
              <div className="notification-actions">
                {!notification.read && (
                  <button
                    className="button button-quiet"
                    disabled={mutation.isPending}
                    onClick={() => void action(notification.id, 'read')}
                  >
                    Read
                  </button>
                )}
                <button
                  className="button button-quiet"
                  disabled={mutation.isPending}
                  onClick={() =>
                    void action(notification.id, 'snooze', {
                      until: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
                    })
                  }
                >
                  Snooze 15m
                </button>
                <button
                  className="button button-quiet"
                  disabled={mutation.isPending}
                  onClick={() => void action(notification.id, 'dismiss')}
                >
                  Dismiss
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
      <button
        className="button button-quiet notification-footer"
        disabled={mutation.isPending}
        onClick={() => void markAllRead()}
      >
        Mark all as read
      </button>
    </div>
  );
}
