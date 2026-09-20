import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import {
  CalendarDays,
  ChevronRight,
  Database,
  Monitor,
  Moon,
  Palette,
  Sun,
  Trash2,
  Upload,
  X,
} from 'lucide-react';
import { useTheme } from 'next-themes';
import { apiBaseUrl, apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';
import {
  clearCustomTheme,
  parseCustomTheme,
  readCustomTheme,
  saveCustomTheme,
  type CustomTheme,
} from '../../theme';
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs';
import { ContainerManager, type ManagedContainer } from '../container-manager';
import {
  accentColors,
  fontSizes,
  getActiveContainers,
  getWeekStart,
  lists,
  weekStartStorageKey,
  type FontSize,
  type WeekStart,
} from '../../data';

type SettingsSection = 'appearance' | 'account' | 'calendar' | 'dev';
type AccountModal = 'email' | 'password' | null;
type SsoProvider = { name: string; displayName: string };
type LinkedSsoAccount = { id: string; provider: string };
const settingsSections: SettingsSection[] = import.meta.env.DEV
  ? ['appearance', 'account', 'calendar', 'dev']
  : ['appearance', 'account', 'calendar'];

export function SettingsTab({ onChanged }: { onChanged?: () => void }) {
  const { theme, setTheme } = useTheme();
  const userId = pb.authStore.record?.id ?? 'anonymous';
  const accentStorageKey = `taskboard-accent-${userId}`;
  const fontSizeStorageKey = `taskboard-font-size-${userId}`;
  const [accentColor, setAccentColor] = useState(
    () => localStorage.getItem(accentStorageKey) ?? accentColors[0].value,
  );
  const [customTheme, setCustomTheme] = useState<CustomTheme | null>(readCustomTheme);
  const [themeImportError, setThemeImportError] = useState('');
  const [themeText, setThemeText] = useState('');
  const [fontSize, setFontSize] = useState<FontSize>(() => {
    const storedSize = localStorage.getItem(fontSizeStorageKey);
    return fontSizes.some((option) => option.value === storedSize)
      ? (storedSize as FontSize)
      : 'medium';
  });
  const [importedCalendars, setImportedCalendars] = useState<string[]>([]);
  const [calendarIOError, setCalendarIOError] = useState('');
  const [calendarIOBusy, setCalendarIOBusy] = useState(false);
  const [weekStart, setWeekStart] = useState<WeekStart>(getWeekStart);
  const [appPasswords, setAppPasswords] = useState<{ id: string; name: string }[]>([]);
  const [newPasswordName, setNewPasswordName] = useState('CalDAV');
  const [newPassword, setNewPassword] = useState<string | null>(null);
  const [appPasswordBusy, setAppPasswordBusy] = useState(false);
  const [appPasswordError, setAppPasswordError] = useState('');
  const [newEmail, setNewEmail] = useState('');
  const [emailBusy, setEmailBusy] = useState(false);
  const [emailMessage, setEmailMessage] = useState('');
  const [emailError, setEmailError] = useState('');
  const [currentPassword, setCurrentPassword] = useState('');
  const [password, setPassword] = useState('');
  const [passwordConfirmation, setPasswordConfirmation] = useState('');
  const [passwordBusy, setPasswordBusy] = useState(false);
  const [passwordMessage, setPasswordMessage] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [accountModal, setAccountModal] = useState<AccountModal>(null);
  const [ssoProviders, setSsoProviders] = useState<SsoProvider[]>([]);
  const [linkedSsoAccounts, setLinkedSsoAccounts] = useState<LinkedSsoAccount[]>([]);
  const [ssoBusy, setSsoBusy] = useState<string | null>(null);
  const [ssoError, setSsoError] = useState('');
  const [activeSection, setActiveSection] = useState<SettingsSection>('appearance');
  const [devSeedBusy, setDevSeedBusy] = useState(false);
  const [devSeedMessage, setDevSeedMessage] = useState('');
  const [devSeedError, setDevSeedError] = useState('');
  const visibilityStorageKey = `taskboard-hidden-containers-${userId}`;
  const [hiddenContainerIds, setHiddenContainerIds] = useState<string[]>(() => {
    try {
      return JSON.parse(localStorage.getItem(visibilityStorageKey) ?? '[]') as string[];
    } catch {
      return [];
    }
  });
  const carddavBaseUrl = `${apiBaseUrl}/carddav`;

  useEffect(() => {
    const updateCustomTheme = () => setCustomTheme(readCustomTheme());
    window.addEventListener('taskboard-custom-theme-change', updateCustomTheme);
    return () => window.removeEventListener('taskboard-custom-theme-change', updateCustomTheme);
  }, []);

  useEffect(() => {
    if (!customTheme) document.documentElement.style.setProperty('--primary', accentColor);
    localStorage.setItem(accentStorageKey, accentColor);
  }, [accentColor, accentStorageKey, customTheme]);

  useEffect(() => {
    const selectedSize = fontSizes.find((option) => option.value === fontSize) ?? fontSizes[1];
    document.documentElement.style.setProperty('--app-font-size', selectedSize.size);
    localStorage.setItem(fontSizeStorageKey, fontSize);
  }, [fontSize, fontSizeStorageKey]);

  useEffect(() => {
    void apiFetch('/api/auth/app-passwords', {
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    })
      .then(async (response) => {
        if (!response.ok) throw new Error();
        return (await response.json()) as { id: string; name: string }[];
      })
      .then(setAppPasswords)
      .catch(() => setAppPasswordError('Could not load app passwords.'));
  }, []);

  const loadSsoAccounts = async () => {
    const userId = pb.authStore.record?.id;
    if (!userId) return;
    const [methods, accounts] = await Promise.all([
      pb.collection('users').listAuthMethods(),
      pb.collection('users').listExternalAuths(userId),
    ]);
    setSsoProviders(
      methods.oauth2.enabled
        ? methods.oauth2.providers.map(({ name, displayName }) => ({ name, displayName }))
        : [],
    );
    setLinkedSsoAccounts(accounts.map(({ id, provider }) => ({ id, provider })));
  };

  useEffect(() => {
    void loadSsoAccounts().catch(() => {
      setSsoProviders([]);
      setSsoError('Could not load SSO accounts.');
    });
  }, []);

  const linkSso = async (provider: SsoProvider) => {
    setSsoBusy(provider.name);
    setSsoError('');
    try {
      await pb.collection('users').authWithOAuth2({ provider: provider.name });
      await loadSsoAccounts();
    } catch {
      setSsoError(`Could not link ${provider.displayName}. Try again.`);
    } finally {
      setSsoBusy(null);
    }
  };

  const unlinkSso = async (provider: SsoProvider) => {
    const userId = pb.authStore.record?.id;
    if (!userId) return;
    setSsoBusy(provider.name);
    setSsoError('');
    try {
      await pb.collection('users').unlinkExternalAuth(userId, provider.name);
      setLinkedSsoAccounts((accounts) =>
        accounts.filter((account) => account.provider !== provider.name),
      );
    } catch {
      setSsoError(`Could not unlink ${provider.displayName}. Try again.`);
    } finally {
      setSsoBusy(null);
    }
  };

  const createAppPassword = async () => {
    setAppPasswordBusy(true);
    setAppPasswordError('');
    setNewPassword(null);
    try {
      const response = await apiFetch('/api/auth/app-passwords', {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${pb.authStore.token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ name: newPasswordName }),
      });
      if (!response.ok) throw new Error();
      const created = (await response.json()) as { id: string; name: string; secret: string };
      setAppPasswords((current) => [{ id: created.id, name: created.name }, ...current]);
      setNewPassword(created.secret);
    } catch {
      setAppPasswordError('Could not create app password.');
    } finally {
      setAppPasswordBusy(false);
    }
  };

  const revokeAppPassword = async (id: string) => {
    setAppPasswordError('');
    try {
      const response = await apiFetch(`/api/auth/app-passwords/${id}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
      });
      if (!response.ok) throw new Error();
      setAppPasswords((current) => current.filter((password) => password.id !== id));
    } catch {
      setAppPasswordError('Could not revoke app password.');
    }
  };

  const requestEmailChange = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const email = newEmail.trim();
    const currentEmail = String(pb.authStore.record?.email ?? '');
    setEmailBusy(true);
    setEmailMessage('');
    setEmailError('');
    try {
      if (email.toLowerCase() === currentEmail.toLowerCase()) {
        throw new Error('That is already your current email address.');
      }
      await pb.collection('users').requestEmailChange(email);
      setNewEmail('');
      setEmailMessage('Check your new email address for a confirmation link.');
    } catch (error) {
      setEmailError(
        error instanceof Error && error.message
          ? error.message
          : 'Could not request an email change. Check the address and try again.',
      );
    } finally {
      setEmailBusy(false);
    }
  };

  const changePassword = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setPasswordBusy(true);
    setPasswordMessage('');
    setPasswordError('');
    try {
      if (password !== passwordConfirmation) {
        throw new Error('New passwords do not match.');
      }
      const record = pb.authStore.record;
      const email = String(record?.email ?? '');
      if (!record?.id || !email) throw new Error('Your session has expired. Please sign in again.');
      await pb.collection('users').update(record.id, {
        oldPassword: currentPassword,
        password,
        passwordConfirm: passwordConfirmation,
      });
      try {
        await pb.collection('users').authWithPassword(email, password);
      } catch {
        pb.authStore.clear();
        return;
      }
      setCurrentPassword('');
      setPassword('');
      setPasswordConfirmation('');
      setPasswordMessage('Your password has been changed.');
    } catch (error) {
      setPasswordError(
        error instanceof Error && error.message
          ? error.message
          : 'Could not change your password. Check your current password and try again.',
      );
    } finally {
      setPasswordBusy(false);
    }
  };

  const applyThemeText = (text: string) => {
    setThemeImportError('');
    try {
      const imported = parseCustomTheme(JSON.parse(text));
      if (!imported) throw new Error('invalid theme');
      saveCustomTheme(imported);
      setCustomTheme(imported);
      setTheme(imported.appearance);
      setThemeText('');
    } catch {
      setThemeImportError('That is not a valid Taskboard theme.');
    }
  };

  const handleThemeImport = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => applyThemeText(String(reader.result));
    reader.onerror = () => setThemeImportError('Could not read that theme file.');
    reader.readAsText(file);
  };

  const handleImport = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    const activeContainers = getActiveContainers(lists);
    const calendar = activeContainers.find((container) => container.kind === 'calendar');
    event.target.value = '';
    if (!calendar || files.length === 0) return;
    setCalendarIOBusy(true);
    setCalendarIOError('');
    try {
      const results: string[] = [];
      for (const file of files) {
        const form = new FormData();
        form.append('calendar', calendar.id);
        const list = activeContainers.find((container) => container.kind === 'list');
        if (list) form.append('list', list.id);
        form.append('file', file);
        const response = await apiFetch('/api/calendars/import', {
          method: 'POST',
          headers: { Authorization: `Bearer ${pb.authStore.token}` },
          body: form,
        });
        if (!response.ok) throw new Error(`Could not import ${file.name}.`);
        const result = (await response.json()) as { imported: number };
        results.push(`${file.name}: ${result.imported} event${result.imported === 1 ? '' : 's'}`);
      }
      setImportedCalendars((current) => [...results, ...current]);
      onChanged?.();
    } catch (error) {
      setCalendarIOError(error instanceof Error ? error.message : 'Could not import calendar.');
    } finally {
      setCalendarIOBusy(false);
    }
  };

  const managedContainers: ManagedContainer[] = lists.map((container) => ({
    ...container,
    visible: !hiddenContainerIds.includes(container.id),
  }));
  const updateContainer = async (
    container: ManagedContainer,
    changes: Pick<ManagedContainer, 'name' | 'color' | 'description'>,
  ) => {
    await pb
      .collection(container.kind === 'list' ? 'lists' : 'calendars')
      .update(container.id, changes);
    onChanged?.();
  };
  const setContainerVisible = (_container: ManagedContainer, visible: boolean) => {
    const next = visible
      ? hiddenContainerIds.filter((id) => id !== _container.id)
      : [...new Set([...hiddenContainerIds, _container.id])];
    setHiddenContainerIds(next);
    localStorage.setItem(visibilityStorageKey, JSON.stringify(next));
    onChanged?.();
  };
  const setContainerArchived = async (container: ManagedContainer, archived: boolean) => {
    await pb
      .collection(container.kind === 'list' ? 'lists' : 'calendars')
      .update(container.id, { archived });
    const next = archived
      ? [...new Set([...hiddenContainerIds, container.id])]
      : hiddenContainerIds.filter((id) => id !== container.id);
    setHiddenContainerIds(next);
    localStorage.setItem(visibilityStorageKey, JSON.stringify(next));
    onChanged?.();
  };
  const removeContainer = async (container: ManagedContainer, action: 'delete' | 'archive') => {
    if (action === 'archive') {
      await setContainerArchived(container, true);
      return;
    }
    await pb.collection(container.kind === 'list' ? 'lists' : 'calendars').delete(container.id);
    onChanged?.();
  };

  const loadDevDefaults = async () => {
    if (!import.meta.env.DEV || devSeedBusy) return;
    const owner = pb.authStore.record?.id;
    if (!owner) return;
    setDevSeedBusy(true);
    setDevSeedMessage('');
    setDevSeedError('');
    try {
      const findOrCreate = async (
        collection: string,
        name: string,
        values: Record<string, unknown>,
      ) => {
        const records = await pb
          .collection(collection)
          .getFullList<{ id: string; name?: string; title?: string }>({
            filter: `owner = "${owner}"`,
          });
        const existing = records.find((record) => (record.name ?? record.title) === name);
        if (existing) return existing;
        return pb.collection(collection).create(values);
      };
      const devList = await findOrCreate('lists', 'Dev Tasks', {
        owner,
        name: 'Dev Tasks',
        description: 'Tasks loaded by the development data tool.',
        color: '#87c4a8',
      });
      const devListTwo = await findOrCreate('lists', 'Dev Errands', {
        owner,
        name: 'Dev Errands',
        description: 'More sample tasks for development.',
        color: '#c66b32',
      });
      const devCalendar = await findOrCreate('calendars', 'Dev Events', {
        owner,
        name: 'Dev Events',
        description: 'Events loaded by the development data tool.',
        color: '#3b6ea8',
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      });
      const devCalendarTwo = await findOrCreate('calendars', 'Dev Plans', {
        owner,
        name: 'Dev Plans',
        description: 'More sample events for development.',
        color: '#7657a8',
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      });
      const addressBook = await findOrCreate('address_books', 'Dev Contacts', {
        owner,
        name: 'Dev Contacts',
        description: 'Contacts loaded by the development data tool.',
        color: '#7657a8',
      });

      const contacts = [
        ['Ada Lovelace', 'ada@example.test', '+1 555 0101'],
        ['Grace Hopper', 'grace@example.test', '+1 555 0102'],
        ['Linus Torvalds', 'linus@example.test', '+1 555 0103'],
        ['Margaret Hamilton', 'margaret@example.test', '+1 555 0104'],
      ];
      const existingContacts = await pb
        .collection('contacts')
        .getFullList<{ formatted_name: string }>({
          filter: `owner = "${owner}"`,
        });
      for (const [formattedName, email, phone] of contacts) {
        if (existingContacts.some((contact) => contact.formatted_name === formattedName)) continue;
        await pb.collection('contacts').create({
          owner,
          address_book: addressBook.id,
          uid: crypto.randomUUID(),
          formatted_name: formattedName,
          given_name: formattedName.split(' ')[0],
          family_name: formattedName.split(' ').slice(1).join(' '),
          email,
          phone,
        });
      }

      const existingEvents = await pb
        .collection('events')
        .getFullList<{ title: string }>({ filter: `owner = "${owner}"` });
      const existingTodos = await pb
        .collection('todos')
        .getFullList<{ title: string }>({ filter: `owner = "${owner}"` });
      const start = new Date();
      start.setDate(start.getDate() + 1);
      start.setHours(9, 0, 0, 0);
      for (let index = 0; index < 10; index += 1) {
        const date = new Date(start);
        date.setDate(start.getDate() + index * 2);
        const iso = date.toISOString();
        const title = `Dev ${index % 2 === 0 ? 'event' : 'todo'} ${index + 1}`;
        if (index % 2 === 0) {
          if (existingEvents.some((event) => event.title === title)) continue;
          const end = new Date(date.getTime() + 60 * 60 * 1000);
          await pb.collection('events').create({
            owner,
            title,
            description: 'Sample event loaded by the development data tool.',
            uid: crypto.randomUUID(),
            dtstamp: new Date().toISOString(),
            start_date: iso,
            start_local: iso.slice(0, 16),
            end_date: end.toISOString(),
            end_local: end.toISOString().slice(0, 16),
            calendar: index % 4 === 0 ? devCalendar.id : devCalendarTwo.id,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            time_mode: 'zoned',
          });
        } else {
          if (existingTodos.some((todo) => todo.title === title)) continue;
          await pb.collection('todos').create({
            owner,
            title,
            description: 'Sample todo loaded by the development data tool.',
            uid: crypto.randomUUID(),
            dtstamp: new Date().toISOString(),
            start_date: iso,
            due_date: iso,
            start_local: iso.slice(0, 16),
            due_local: iso.slice(0, 16),
            list: index % 4 === 1 ? devList.id : devListTwo.id,
            completed: false,
            status: 'NEEDS-ACTION',
            time_mode: 'zoned',
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
          });
        }
      }
      setDevSeedMessage('Defaults loaded for the next three weeks.');
      onChanged?.();
    } catch {
      setDevSeedError('Could not load development defaults.');
    } finally {
      setDevSeedBusy(false);
    }
  };

  const exportCalendar = async (id: string, name: string) => {
    setCalendarIOBusy(true);
    setCalendarIOError('');
    try {
      const response = await apiFetch(`/api/calendars/${id}/export.ics`, {
        headers: { Authorization: `Bearer ${pb.authStore.token}` },
      });
      if (!response.ok) throw new Error(`Could not export ${name}.`);
      const url = URL.createObjectURL(await response.blob());
      const link = document.createElement('a');
      link.href = url;
      link.download = `${name}.ics`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      setCalendarIOError(error instanceof Error ? error.message : 'Could not export calendar.');
    } finally {
      setCalendarIOBusy(false);
    }
  };

  const selectBuiltInTheme = (nextTheme: 'light' | 'dark' | 'system') => {
    if (customTheme) {
      clearCustomTheme();
      setCustomTheme(null);
    }
    setTheme(nextTheme);
  };

  return (
    <>
      <header className="page-header">
        <h1>Settings</h1>
      </header>
      <Tabs
        value={activeSection}
        onValueChange={(value) => setActiveSection(value as SettingsSection)}
        className="settings-tabs"
      >
        <TabsList className="create-tabs-list settings-tabs-list">
          <span
            className="create-tabs-track settings-tabs-track"
            style={{
              width: `calc((100% - 11px) / ${settingsSections.length})`,
              transform: `translateX(calc(${settingsSections.indexOf(activeSection)} * (100% + 3px)))`,
            }}
            aria-hidden="true"
          />
          <TabsTrigger value="appearance">Appearance</TabsTrigger>
          <TabsTrigger value="account">Account</TabsTrigger>
          <TabsTrigger value="calendar">Calendar</TabsTrigger>
          {import.meta.env.DEV && <TabsTrigger value="dev">Dev</TabsTrigger>}
        </TabsList>
      </Tabs>
      <section className="settings-card">
        {activeSection === 'appearance' && (
          <>
            <div className="settings-section">
              <div>
                <h2>Theme</h2>
                <p>Choose how Taskboard looks on your device.</p>
              </div>
              <div className="theme-toggle" role="group" aria-label="Theme">
                <button
                  className={theme === 'light' ? 'active' : ''}
                  onClick={() => selectBuiltInTheme('light')}
                  aria-pressed={theme === 'light'}
                >
                  <Sun size={16} /> Light
                </button>
                <button
                  className={theme === 'dark' ? 'active' : ''}
                  onClick={() => selectBuiltInTheme('dark')}
                  aria-pressed={theme === 'dark'}
                >
                  <Moon size={16} /> Dark
                </button>
                <button
                  className={theme === 'system' ? 'active' : ''}
                  onClick={() => selectBuiltInTheme('system')}
                  aria-pressed={theme === 'system'}
                >
                  <Monitor size={16} /> Device
                </button>
              </div>
            </div>
            <div className="settings-section custom-theme-settings">
              <div>
                <h2>Custom theme</h2>
                <p>
                  {customTheme
                    ? `${customTheme.name}${customTheme.author ? ` by ${customTheme.author}` : ''}`
                    : 'Import a theme JSON file.'}
                </p>
                {themeImportError && <small className="settings-error">{themeImportError}</small>}
              </div>
              <div className="custom-theme-controls">
                <textarea
                  className="custom-theme-input"
                  value={themeText}
                  onChange={(event) => setThemeText(event.target.value)}
                  placeholder="Paste theme JSON here"
                  aria-label="Paste custom theme JSON"
                  rows={4}
                />
                <div className="theme-toggle">
                  <button
                    type="button"
                    className="button button-quiet"
                    onClick={() => applyThemeText(themeText)}
                    disabled={!themeText.trim()}
                  >
                    Apply pasted theme
                  </button>
                  <label className="button button-quiet">
                    <Upload size={16} /> Import file
                    <input
                      type="file"
                      accept="application/json,.json"
                      onChange={handleThemeImport}
                      hidden
                    />
                  </label>
                  {customTheme && (
                    <button
                      type="button"
                      className="button button-quiet"
                      onClick={() => {
                        clearCustomTheme();
                        setCustomTheme(null);
                      }}
                    >
                      Reset
                    </button>
                  )}
                </div>
              </div>
            </div>
            <div className="settings-section font-size-settings">
              <div>
                <h2>Font size</h2>
                <p>Adjust the text size across Taskboard.</p>
              </div>
              <div className="theme-toggle" role="group" aria-label="Font size">
                {fontSizes.map((option) => (
                  <button
                    key={option.value}
                    className={fontSize === option.value ? 'active' : ''}
                    onClick={() => setFontSize(option.value)}
                    aria-pressed={fontSize === option.value}
                  >
                    {option.name}
                  </button>
                ))}
              </div>
            </div>
            <div className="settings-section accent-settings">
              <div>
                <h2>Accent color</h2>
                <p>Choose the color used for your Taskboard accents.</p>
              </div>
              <div className="accent-picker" role="group" aria-label="Accent color">
                {accentColors.map((color) => (
                  <button
                    key={color.value}
                    className={accentColor === color.value ? 'active' : ''}
                    style={{ backgroundColor: color.value }}
                    onClick={() => setAccentColor(color.value)}
                    aria-label={color.name}
                    aria-pressed={accentColor === color.value}
                  />
                ))}
                <label
                  className={`custom-color-picker ${!accentColors.some((color) => color.value === accentColor) ? 'active' : ''}`}
                  style={{ backgroundColor: accentColor }}
                  aria-label="Custom accent color"
                >
                  <Palette size={14} aria-hidden="true" />
                  <input
                    type="color"
                    value={accentColor}
                    onChange={(event) => setAccentColor(event.target.value)}
                    aria-label="Choose a custom accent color"
                  />
                </label>
              </div>
            </div>
          </>
        )}
        {activeSection === 'account' && (
          <div className="settings-section account-settings">
            <div className="account-header">
              <div>
                <h2>Account</h2>
                <p>{pb.authStore.record?.email}</p>
              </div>
              <button className="button button-quiet" onClick={() => pb.authStore.clear()}>
                Log out
              </button>
            </div>
            <div className="account-actions">
              <button
                className="account-action"
                type="button"
                onClick={() => setAccountModal('email')}
              >
                <span>
                  <strong>Change email</strong>
                  <small>We will send a confirmation link to your new address.</small>
                </span>
                <ChevronRight size={18} aria-hidden="true" />
              </button>
              <button
                className="account-action"
                type="button"
                onClick={() => setAccountModal('password')}
              >
                <span>
                  <strong>Change password</strong>
                  <small>Use a password you do not use anywhere else.</small>
                </span>
                <ChevronRight size={18} aria-hidden="true" />
              </button>
            </div>
            {ssoProviders.length > 0 && (
              <div className="account-sso">
                <div>
                  <h2>Single sign-on</h2>
                  <p>Link a work or personal SSO account for faster sign-in.</p>
                </div>
                <div className="sso-account-list">
                  {ssoProviders.map((provider) => {
                    const linked = linkedSsoAccounts.some(
                      (account) => account.provider === provider.name,
                    );
                    return (
                      <div className="sso-account-row" key={provider.name}>
                        <span>
                          <strong>{provider.displayName}</strong>
                          <small>{linked ? 'Linked' : 'Not linked'}</small>
                        </span>
                        <button
                          className="button button-quiet"
                          type="button"
                          disabled={ssoBusy !== null}
                          onClick={() => void (linked ? unlinkSso(provider) : linkSso(provider))}
                        >
                          {ssoBusy === provider.name
                            ? linked
                              ? 'Unlinking…'
                              : 'Opening SSO…'
                            : linked
                              ? 'Unlink'
                              : `Link ${provider.displayName}`}
                        </button>
                      </div>
                    );
                  })}
                </div>
                {ssoError && <p className="error-text">{ssoError}</p>}
              </div>
            )}
          </div>
        )}
        {activeSection === 'calendar' && (
          <>
            <div className="settings-section week-start-settings">
              <div>
                <h2>Week starts on</h2>
                <p>Choose the first day shown in calendar weeks.</p>
              </div>
              <div className="theme-toggle" role="group" aria-label="Week starts on">
                <button
                  className={weekStart === 'sunday' ? 'active' : ''}
                  onClick={() => {
                    localStorage.setItem(weekStartStorageKey, 'sunday');
                    setWeekStart('sunday');
                  }}
                  aria-pressed={weekStart === 'sunday'}
                >
                  Sunday
                </button>
                <button
                  className={weekStart === 'monday' ? 'active' : ''}
                  onClick={() => {
                    localStorage.setItem(weekStartStorageKey, 'monday');
                    setWeekStart('monday');
                  }}
                  aria-pressed={weekStart === 'monday'}
                >
                  Monday
                </button>
              </div>
            </div>
            <div className="settings-section calendar-settings">
              <ContainerManager
                containers={managedContainers}
                onUpdate={updateContainer}
                onVisibilityChange={setContainerVisible}
                onArchiveChange={setContainerArchived}
                onRemove={removeContainer}
              />
            </div>
            <div className="settings-section calendar-settings">
              <div>
                <h2>Calendars</h2>
                <p>Import an iCalendar file or connect a calendar through CalDAV.</p>
                {calendarIOError && <p className="error-text">{calendarIOError}</p>}
                {importedCalendars.length > 0 && (
                  <div className="imported-calendars">
                    {importedCalendars.map((calendar) => (
                      <span key={calendar}>
                        <CalendarDays size={13} />
                        {calendar}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <label className="button button-quiet import-button">
                  <Upload size={16} /> {calendarIOBusy ? 'Working…' : 'Import calendar'}
                  <input
                    type="file"
                    accept=".ics,.ical,text/calendar"
                    multiple
                    onChange={(event) => void handleImport(event)}
                    disabled={calendarIOBusy}
                  />
                </label>
                {getActiveContainers(lists)
                  .filter((container) => container.kind === 'calendar')
                  .map((calendar) => (
                    <button
                      key={calendar.id}
                      className="button button-quiet"
                      disabled={calendarIOBusy}
                      onClick={() => void exportCalendar(calendar.id, calendar.name)}
                    >
                      Export {calendar.name}
                    </button>
                  ))}
              </div>
            </div>
            <div className="settings-section calendar-sync-settings">
              <div>
                <h2>Contact sync</h2>
                <p>
                  Use this URL in your CardDAV client. Sign in with your account email and an app
                  password. CalDAV calendar sync is not available yet.
                </p>
                <div className="sync-urls">
                  <label>
                    CardDAV server URL<code>{carddavBaseUrl}</code>
                  </label>
                </div>
                <div className="app-passwords">
                  <strong>DAV app passwords</strong>
                  <p>
                    App passwords can be used instead of your real password. You will only see the
                    secret once.
                  </p>
                  <div className="app-password-create">
                    <input
                      aria-label="App password name"
                      value={newPasswordName}
                      onChange={(event) => setNewPasswordName(event.target.value)}
                      placeholder="Name, for example iPhone"
                    />
                    <button
                      className="button button-quiet"
                      onClick={() => void createAppPassword()}
                      disabled={appPasswordBusy}
                    >
                      {appPasswordBusy ? 'Generating…' : 'Generate password'}
                    </button>
                  </div>
                  {newPassword && (
                    <p className="new-app-password">
                      Copy this now: <code>{newPassword}</code>
                    </p>
                  )}
                  {appPasswordError && <p className="error-text">{appPasswordError}</p>}
                  {appPasswords.map((password) => (
                    <div className="app-password-row" key={password.id}>
                      <span>{password.name || 'Unnamed app password'}</span>
                      <button
                        aria-label={`Revoke ${password.name || 'app password'}`}
                        onClick={() => void revokeAppPassword(password.id)}
                      >
                        <Trash2 size={14} />
                      </button>
                    </div>
                  ))}
                </div>
              </div>
              <span className="status-pill">
                <span /> CardDAV available
              </span>
            </div>
          </>
        )}
        {import.meta.env.DEV && activeSection === 'dev' && (
          <div className="settings-section">
            <div>
              <h2>Development data</h2>
              <p>
                Create sample contacts, calendars, lists, events, and todos dated over the next
                three weeks.
              </p>
            </div>
            <div>
              <button
                className="button button-primary"
                onClick={() => void loadDevDefaults()}
                disabled={devSeedBusy}
              >
                <Database size={16} /> {devSeedBusy ? 'Loading…' : 'Load defaults'}
              </button>
              {devSeedMessage && (
                <p className="form-message" role="status">
                  {devSeedMessage}
                </p>
              )}
              {devSeedError && (
                <p className="form-error" role="alert">
                  {devSeedError}
                </p>
              )}
            </div>
          </div>
        )}
      </section>
      {accountModal && (
        <div
          className="modal-backdrop"
          role="presentation"
          onMouseDown={() => setAccountModal(null)}
        >
          <div
            className="composer account-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="account-modal-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <div className="composer-header">
              <div>
                <h2 id="account-modal-title">
                  {accountModal === 'email' ? 'Change email' : 'Change password'}
                </h2>
                <p className="modal-subtitle">
                  {accountModal === 'email'
                    ? 'We will send a confirmation link to your new address.'
                    : 'Use a password you do not use anywhere else.'}
                </p>
              </div>
              <button
                type="button"
                className="icon-button"
                onClick={() => setAccountModal(null)}
                aria-label="Close"
              >
                <X size={19} />
              </button>
            </div>
            {accountModal === 'email' ? (
              <form className="account-form" onSubmit={(event) => void requestEmailChange(event)}>
                <label>
                  New email
                  <input
                    type="email"
                    required
                    autoComplete="email"
                    value={newEmail}
                    onChange={(event) => setNewEmail(event.target.value)}
                  />
                </label>
                {emailError && (
                  <p className="form-error" role="alert">
                    {emailError}
                  </p>
                )}
                {emailMessage && (
                  <p className="form-message" role="status">
                    {emailMessage}
                  </p>
                )}
                <button className="button button-primary" disabled={emailBusy}>
                  {emailBusy ? 'Sending…' : 'Change email'}
                </button>
              </form>
            ) : (
              <form className="account-form" onSubmit={(event) => void changePassword(event)}>
                <label>
                  Current password
                  <input
                    type="password"
                    required
                    autoComplete="current-password"
                    value={currentPassword}
                    onChange={(event) => setCurrentPassword(event.target.value)}
                  />
                </label>
                <label>
                  New password
                  <input
                    type="password"
                    required
                    minLength={8}
                    autoComplete="new-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                  />
                </label>
                <label>
                  Confirm new password
                  <input
                    type="password"
                    required
                    minLength={8}
                    autoComplete="new-password"
                    value={passwordConfirmation}
                    onChange={(event) => setPasswordConfirmation(event.target.value)}
                  />
                </label>
                {passwordError && (
                  <p className="form-error" role="alert">
                    {passwordError}
                  </p>
                )}
                {passwordMessage && (
                  <p className="form-message" role="status">
                    {passwordMessage}
                  </p>
                )}
                <button className="button button-primary" disabled={passwordBusy}>
                  {passwordBusy ? 'Changing…' : 'Change password'}
                </button>
              </form>
            )}
          </div>
        </div>
      )}
    </>
  );
}
