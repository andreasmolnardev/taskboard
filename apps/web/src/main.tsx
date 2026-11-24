import { OFFLINE_ENABLED } from './offline';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ThemeProvider } from 'next-themes';
import { createRoot } from 'react-dom/client';
import { TooltipProvider } from './components/ui/tooltip';
import { App } from './app';
import './styles.css';

const queryClient = new QueryClient();

createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={queryClient}>
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem themes={['light', 'dark']}>
      <TooltipProvider>
        <App />
      </TooltipProvider>
    </ThemeProvider>
  </QueryClientProvider>,
);

if (OFFLINE_ENABLED && import.meta.env.PROD && 'serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    void navigator.serviceWorker.register('/sw.js').catch((error) => {
      console.warn('Could not enable offline launch.', error);
    });
  });
}
