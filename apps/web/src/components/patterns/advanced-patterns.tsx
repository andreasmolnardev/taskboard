import { useCallback, useEffect, useMemo, useRef, useState, type ComponentType, type ReactNode } from 'react';
import { MessageCircle, Reply, X } from 'lucide-react';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';

export function EmptyState({
  title,
  description,
  icon: Icon = MessageCircle,
  action,
}: {
  title: string;
  description?: string;
  icon?: ComponentType<{ className?: string }>;
  action?: ReactNode;
}) {
  return (
    <div className="grid min-h-48 place-items-center rounded-lg border border-dashed p-8 text-center">
      <div className="grid justify-items-center gap-3">
        <span className="grid size-11 place-items-center rounded-full bg-muted"><Icon className="size-5 text-muted-foreground" /></span>
        <div className="space-y-1"><h3 className="font-medium">{title}</h3>{description && <p className="max-w-md text-sm text-muted-foreground">{description}</p>}</div>
        {action}
      </div>
    </div>
  );
}

export function LoadingState({ label = 'Loading', rows = 3 }: { label?: string; rows?: number }) {
  return (
    <div className="grid gap-3" role="status" aria-label={label}>
      {Array.from({ length: rows }, (_, index) => <Skeleton key={index} className="h-16 w-full" />)}
      <span className="sr-only">{label}…</span>
    </div>
  );
}

export function DashboardWidget({
  title,
  description,
  action,
  children,
  className,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="space-y-1"><CardTitle>{title}</CardTitle>{description && <CardDescription>{description}</CardDescription>}</div>
        {action}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

export function CollectionView<Item>({
  items,
  getKey,
  groupBy,
  renderItem,
  empty = <EmptyState title="Nothing here yet" description="Add an item to get started." />,
}: {
  items: readonly Item[];
  getKey: (item: Item) => string;
  groupBy?: (item: Item) => string;
  renderItem: (item: Item) => ReactNode;
  empty?: ReactNode;
}) {
  const groups = useMemo(() => {
    const result = new Map<string, Item[]>();
    for (const item of items) {
      const name = groupBy?.(item) ?? '';
      result.set(name, [...(result.get(name) ?? []), item]);
    }
    return result;
  }, [groupBy, items]);
  if (!items.length) return empty;
  return (
    <div className="grid gap-6">
      {[...groups.entries()].map(([group, members]) => (
        <section key={group || 'all'} className="grid gap-3">
          {group && <h2 className="text-sm font-medium text-muted-foreground">{group}</h2>}
          <ul className="grid gap-2">{members.map((item) => <li key={getKey(item)}>{renderItem(item)}</li>)}</ul>
        </section>
      ))}
    </div>
  );
}

export function InfiniteFeed<Item>({
  items,
  getKey,
  renderItem,
  hasMore,
  loading,
  onLoadMore,
  virtualization,
}: {
  items: readonly Item[];
  getKey: (item: Item) => string;
  renderItem: (item: Item) => ReactNode;
  hasMore: boolean;
  loading: boolean;
  onLoadMore: () => void;
  /** Enables fixed-height windowing for large feeds. Leave unset for variable-height content. */
  virtualization?: { height: number; itemHeight: number; overscan?: number };
}) {
  const sentinel = useRef<HTMLDivElement>(null);
  const requested = useRef(false);
  const [scrollTop, setScrollTop] = useState(0);
  useEffect(() => { if (!loading) requested.current = false; }, [loading]);
  const windowed = virtualization ? calculateVirtualWindow(items.length, scrollTop, virtualization) : undefined;
  const visibleItems = windowed ? items.slice(windowed.start, windowed.end) : items;
  const loadMore = useCallback(() => {
    if (!hasMore || loading || requested.current) return;
    requested.current = true;
    onLoadMore();
  }, [hasMore, loading, onLoadMore]);
  useEffect(() => {
    if (!hasMore || loading || !sentinel.current || typeof IntersectionObserver === 'undefined') return;
    const root = virtualization ? sentinel.current?.closest('[data-feed-scroll]') : null;
    const observer = new IntersectionObserver(([entry]) => { if (entry.isIntersecting) loadMore(); }, { root, rootMargin: '160px' });
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [hasMore, loading, loadMore, virtualization]);
  const content = windowed ? (
    <div
      data-feed-scroll
      className="overflow-auto"
      style={{ height: virtualization!.height }}
      onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}
    >
      <div style={{ height: windowed.totalHeight, position: 'relative' }}>
        {visibleItems.map((item, offset) => {
          const index = windowed.start + offset;
          return <div key={getKey(item)} style={{ position: 'absolute', insetInline: 0, height: virtualization!.itemHeight, transform: `translateY(${index * virtualization!.itemHeight}px)` }}>{renderItem(item)}</div>;
        })}
      </div>
    </div>
  ) : (
    <div className="grid gap-4">{items.map((item) => <div key={getKey(item)}>{renderItem(item)}</div>)}</div>
  );
  return (
    <div className="grid gap-4">
      {content}
      {loading && <LoadingState label="Loading more" rows={2} />}
      {hasMore && <div ref={sentinel} className="flex justify-center"><Button variant="outline" disabled={loading} onClick={loadMore}>{loading ? 'Loading…' : 'Load more'}</Button></div>}
    </div>
  );
}

export function calculateVirtualWindow(
  itemCount: number,
  scrollTop: number,
  options: { height: number; itemHeight: number; overscan?: number },
) {
  const itemHeight = Math.max(1, options.itemHeight);
  const overscan = Math.max(0, options.overscan ?? 3);
  const start = Math.max(0, Math.floor(Math.max(0, scrollTop) / itemHeight) - overscan);
  const end = Math.min(itemCount, Math.ceil((Math.max(0, scrollTop) + Math.max(1, options.height)) / itemHeight) + overscan);
  return { start, end, totalHeight: itemCount * itemHeight };
}

export type ThreadComment = {
  id: string;
  author: string;
  body: string;
  createdAt?: string;
  parentId?: string;
};

export function CommentThread({
  comments,
  onSubmit,
  currentUser,
}: {
  comments: readonly ThreadComment[];
  onSubmit: (body: string, parentId?: string) => void | Promise<void>;
  currentUser?: string;
}) {
  const [body, setBody] = useState('');
  const [replyTo, setReplyTo] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const children = useMemo(() => {
    const groups = new Map<string | undefined, ThreadComment[]>();
    for (const comment of comments) groups.set(comment.parentId, [...(groups.get(comment.parentId) ?? []), comment]);
    return groups;
  }, [comments]);
  const submit = async () => {
    if (!body.trim() || submitting) return;
    setSubmitting(true);
    setSubmitError('');
    try { await onSubmit(body.trim(), replyTo); setBody(''); setReplyTo(undefined); }
    catch (error) { setSubmitError(error instanceof Error ? error.message : 'Could not send this comment.'); }
    finally { setSubmitting(false); }
  };
  const renderComments = (parentId?: string, depth = 0, ancestors = new Set<string>()): ReactNode => (children.get(parentId) ?? []).map((comment) => {
    if (ancestors.has(comment.id)) return null;
    const branch = new Set(ancestors);
    branch.add(comment.id);
    return (
    <article key={comment.id} className="grid gap-2 rounded-lg border p-3" style={{ marginInlineStart: `${Math.min(depth, 4) * 1.25}rem` }}>
      <div className="flex items-center gap-2">
        <Avatar className="size-7"><AvatarFallback className="text-[10px]">{comment.author.slice(0, 2).toUpperCase()}</AvatarFallback></Avatar>
        <span className="text-sm font-medium">{comment.author}</span>
        {comment.createdAt && <time className="text-xs text-muted-foreground">{comment.createdAt}</time>}
        <Button className="ml-auto" size="sm" variant="ghost" onClick={() => setReplyTo(comment.id)}><Reply className="mr-1 size-3" />Reply</Button>
      </div>
      <p className="whitespace-pre-wrap text-sm">{comment.body}</p>
      {renderComments(comment.id, depth + 1, branch)}
    </article>
    );
  });

  return (
    <section className="grid gap-4" aria-label="Comments">
      <div className="grid gap-3">{comments.length ? renderComments() : <EmptyState title="No comments yet" description="Start the discussion." />}</div>
      {replyTo && <div className="flex items-center justify-between text-sm text-muted-foreground"><span>Replying to a comment</span><Button size="icon" variant="ghost" aria-label="Cancel reply" onClick={() => setReplyTo(undefined)}><X className="size-4" /></Button></div>}
      <Textarea value={body} onChange={(event) => setBody(event.target.value)} aria-label={currentUser ? `Comment as ${currentUser}` : 'Write a comment'} placeholder="Write a comment…" />
      {submitError && <p role="alert" className="text-sm text-destructive">{submitError}</p>}
      <div className="flex justify-end"><Button disabled={!body.trim() || submitting} onClick={() => void submit()}>{submitting ? 'Sending…' : replyTo ? 'Reply' : 'Comment'}</Button></div>
    </section>
  );
}

export function MediaViewer({
  open,
  onOpenChange,
  title,
  src,
  kind,
  alt = '',
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  src: string;
  kind: 'image' | 'video' | 'audio';
  alt?: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl">
        <DialogHeader><DialogTitle>{title}</DialogTitle><DialogDescription className="sr-only">Media viewer</DialogDescription></DialogHeader>
        <div className="grid max-h-[75vh] place-items-center overflow-auto rounded-lg bg-black/5 p-2">
          {kind === 'image' ? <img src={src} alt={alt || title} className="max-h-[70vh] max-w-full object-contain" />
            : kind === 'video' ? <video src={src} controls playsInline className="max-h-[70vh] max-w-full"><track kind="captions" /></video>
              : <audio src={src} controls className="w-full"><track kind="captions" /></audio>}
        </div>
      </DialogContent>
    </Dialog>
  );
}
