import { useEffect, useState } from 'react';
import { Bell, X } from 'lucide-react';
import { apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';

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
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [error, setError] = useState('');
  const load = async () => {
    const response = await apiFetch('/api/notifications', {
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    });
    if (!response.ok) throw new Error('Could not load notifications.');
    const items = (await response.json()) as Notification[];
    setNotifications(items);
    setError('');
    onCountChange?.(items.filter((item) => !item.read).length);
  };
  const userId = pb.authStore.record?.id;
  useEffect(() => {
    void load().catch((reason: unknown) => {
      setError(reason instanceof Error ? reason.message : 'Could not load notifications.');
      onCountChange?.(0);
    });
  }, [open, userId]);
  const action = async (id: string, path: string, body?: object) => {
    try {
      const response = await apiFetch(`/api/notifications/${id}/${path}`, {
        method: path === 'read' ? 'PATCH' : 'POST',
        headers: {
          Authorization: `Bearer ${pb.authStore.token}`,
          ...(body ? { 'Content-Type': 'application/json' } : {}),
        },
        ...(body ? { body: JSON.stringify(body) } : {}),
      });
      if (!response.ok) throw new Error('Could not update this notification.');
      await load();
    } catch (reason: unknown) {
      setError(reason instanceof Error ? reason.message : 'Could not update this notification.');
    }
  };
  const markAllRead = async () => {
    try {
      const response = await apiFetch('/api/notifications/read-all', {
        method: 'POST',
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
      });
      if (!response.ok) throw new Error('Could not mark notifications as read.');
      await load();
    } catch (reason: unknown) {
      setError(reason instanceof Error ? reason.message : 'Could not mark notifications as read.');
    }
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
          <strong>{error || 'No notifications'}</strong>
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
                    onClick={() => void action(notification.id, 'read')}
                  >
                    Read
                  </button>
                )}
                <button
                  className="button button-quiet"
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
        onClick={() => void markAllRead()}
      >
        Mark all as read
      </button>
    </div>
  );
}
