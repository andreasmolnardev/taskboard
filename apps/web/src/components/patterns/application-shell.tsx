import type { ComponentType, ReactNode } from 'react';
import { NavigationLinks, type NavigationStyle } from './navigation-links';

export type ShellNavigationItem = {
  id: string;
  label: string;
  icon: ComponentType<{ className?: string }>;
};

export function ApplicationShell({
  brand,
  items,
  activeId,
  navigationStyle = 'sidebar',
  onNavigate,
  children,
  account,
}: {
  brand: ReactNode;
  items: readonly ShellNavigationItem[];
  activeId: string;
  navigationStyle?: NavigationStyle;
  onNavigate: (id: string) => void;
  children: ReactNode;
  account?: ReactNode;
}) {
  const nav = <NavigationLinks items={[...items]} activeId={activeId} layout={navigationStyle} onNavigate={onNavigate} label="Primary navigation" />;

  if (navigationStyle === 'topbar') {
    return <div className="min-h-screen bg-background"><header className="flex min-h-16 flex-wrap items-center justify-between gap-4 border-b px-4 sm:px-6"><div className="font-semibold">{brand}</div>{nav}{account}</header><main className="mx-auto w-full max-w-7xl p-4 sm:p-6">{children}</main></div>;
  }

  if (navigationStyle === 'bottom') {
    return <div className="min-h-screen bg-background pb-20"><header className="flex min-h-14 items-center justify-between border-b px-4"><div className="font-semibold">{brand}</div>{account}</header><main className="mx-auto w-full max-w-7xl p-4 sm:p-6">{children}</main><div className="fixed inset-x-0 bottom-0 z-40 border-t bg-background/95 px-2 py-1 backdrop-blur">{nav}</div></div>;
  }

  const rail = navigationStyle === 'rail';
  return (
    <div className={`min-h-screen bg-background ${rail ? 'md:pl-16' : 'md:pl-64'}`}>
      <aside className={`fixed inset-y-0 left-0 z-30 hidden border-r bg-card md:flex md:flex-col ${rail ? 'w-16 items-center px-2' : 'w-64 p-4'}`}>
        <div className="mb-6 flex min-h-10 max-w-full items-center overflow-hidden font-semibold" aria-label="Application">{brand}</div>
        {nav}
        {account && <div className="mt-auto border-t pt-4">{account}</div>}
      </aside>
      <header className="flex min-h-14 items-center justify-between border-b px-4 md:hidden">
        <div className="font-semibold">{brand}</div>
        <div className="flex items-center gap-3">
          {account}
          <details className="relative">
            <summary className="cursor-pointer list-none rounded-md border px-3 py-1.5 text-sm">Menu</summary>
            <div className="absolute right-0 top-full z-50 mt-2 min-w-52 rounded-lg border bg-popover p-2 shadow-lg">{nav}</div>
          </details>
        </div>
      </header>
      <main className="mx-auto w-full max-w-7xl p-4 sm:p-6">{children}</main>
    </div>
  );
}
