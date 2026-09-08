import { Bell, X } from 'lucide-react';

export function NotificationsPopover({
  closing,
  onClose,
  onAnimationEnd,
}: {
  closing: boolean;
  onClose: () => void;
  onAnimationEnd: (event: React.AnimationEvent<HTMLDivElement>) => void;
}) {
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
      <div className="empty-state notification-empty">
        <Bell size={22} />
        <strong>No notifications</strong>
        <span>Updates will appear here.</span>
      </div>
      <button className="button button-quiet notification-footer" onClick={onClose}>
        Mark all as read
      </button>
    </div>
  );
}
