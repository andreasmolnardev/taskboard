import type { ReactNode } from 'react';
import { Separator } from '@/components/ui/separator';
import { PageHeading } from './page-heading';

export function PageLayout({
  title,
  actions,
  children,
  eyebrow = 'Workspace',
  description = '',
}: {
  title: string;
  description?: string;
  eyebrow?: string;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return <div className="space-y-6"><PageHeading title={title} description={description} eyebrow={eyebrow} action={actions} />{children}</div>;
}

export function FeedLayout({ children, columns = 'single' }: { children: ReactNode; columns?: 'single' | 'two' | 'three' }) {
  const columnStyle = { single: 'grid-cols-1', two: 'grid-cols-1 md:grid-cols-2', three: 'grid-cols-1 md:grid-cols-2 xl:grid-cols-3' }[columns];
  return <div className={`grid items-start gap-4 ${columnStyle}`}>{children}</div>;
}

export function DetailLayout({ main, aside, asideLabel = 'Related information' }: { main: ReactNode; aside?: ReactNode; asideLabel?: string }) {
  return <div className={`grid items-start gap-6 ${aside ? 'lg:grid-cols-[minmax(0,1fr)_20rem]' : ''}`}><main className="min-w-0">{main}</main>{aside && <aside aria-label={asideLabel} className="min-w-0">{aside}</aside>}</div>;
}

export function MasterDetailLayout({ master, detail, masterLabel = 'Items', detailLabel = 'Selected item' }: { master: ReactNode; detail?: ReactNode; masterLabel?: string; detailLabel?: string }) {
  return <div className="grid min-h-[32rem] overflow-hidden rounded-xl border md:grid-cols-[minmax(16rem,0.8fr)_minmax(0,1.5fr)]"><section aria-label={masterLabel} className="min-w-0 border-b p-4 md:border-r md:border-b-0">{master}</section><section aria-label={detailLabel} className="min-w-0 p-4">{detail ?? <p className="grid min-h-48 place-items-center text-sm text-muted-foreground">Choose an item to see its details.</p>}</section></div>;
}

export function CollectionLayout({ header, children, footer }: { header: ReactNode; children: ReactNode; footer?: ReactNode }) {
  return <section className="overflow-hidden rounded-xl border bg-card"><div className="p-4 sm:p-6">{header}</div><Separator />{children}{footer && <><Separator /><div className="p-4 sm:p-6">{footer}</div></>}</section>;
}

export function SettingsLayout({ sections, active, onSelect, children }: { sections: readonly { id: string; label: string }[]; active: string; onSelect: (id: string) => void; children: ReactNode }) {
  return <div className="grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)]"><nav aria-label="Settings sections" className="flex gap-2 overflow-x-auto lg:flex-col">{sections.map((section) => <button key={section.id} type="button" aria-current={active === section.id ? 'page' : undefined} onClick={() => onSelect(section.id)} className={`rounded-md px-3 py-2 text-left text-sm ${active === section.id ? 'bg-muted font-medium' : 'text-muted-foreground hover:bg-muted/60'}`}>{section.label}</button>)}</nav><section className="min-w-0">{children}</section></div>;
}

export function ManagementLayout({ toolbar, children, footer }: { toolbar?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  return <div className="grid gap-4">{toolbar && <div className="flex flex-wrap items-center justify-between gap-3">{toolbar}</div>}<div className="min-w-0 overflow-hidden rounded-xl border bg-card">{children}</div>{footer && <footer className="flex flex-wrap items-center justify-between gap-3">{footer}</footer>}</div>;
}

export function ImmersiveLayout({ children, onExit, exitLabel = 'Exit full screen' }: { children: ReactNode; onExit?: () => void; exitLabel?: string }) {
  return <main className="fixed inset-0 z-50 overflow-auto bg-background p-4 sm:p-8">{onExit && <button type="button" className="absolute right-4 top-4 rounded-md p-2 text-sm text-muted-foreground hover:bg-muted" onClick={onExit}>{exitLabel}</button>}<div className="mx-auto min-h-full max-w-7xl">{children}</div></main>;
}

export function PersistentRegion({ children, label, placement = 'bottom' }: { children: ReactNode; label: string; placement?: 'inline' | 'bottom' | 'top' | 'floating' }) {
  const placementStyle = {
    inline: '',
    bottom: 'fixed inset-x-0 bottom-0 z-40 border-t bg-card shadow-lg',
    top: 'fixed inset-x-0 top-0 z-40 border-b bg-card shadow-lg',
    floating: 'fixed bottom-4 right-4 z-40 rounded-xl border bg-card shadow-xl',
  }[placement];
  return <aside aria-label={label} data-persistent-region className={placementStyle}>{children}</aside>;
}
