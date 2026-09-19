import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { CalendarDays, Monitor, Moon, Palette, Sun, Trash2, Upload } from 'lucide-react';
import { useTheme } from 'next-themes';
import { apiBaseUrl, apiFetch } from '../../api/client';
import { pb } from '../../api/pocketbase';
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

type SettingsSection = 'appearance' | 'account' | 'calendar';
const settingsSections: SettingsSection[] = ['appearance', 'account', 'calendar'];

export function SettingsTab({ onChanged }: { onChanged?: () => void }) {
  const { theme, setTheme } = useTheme();
  const userId = pb.authStore.record?.id ?? 'anonymous';
  const accentStorageKey = `taskboard-accent-${userId}`;
  const fontSizeStorageKey = `taskboard-font-size-${userId}`;
  const [accentColor, setAccentColor] = useState(
    () => localStorage.getItem(accentStorageKey) ?? accentColors[0].value,
  );
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
  const [activeSection, setActiveSection] = useState<SettingsSection>('appearance');
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
    document.documentElement.style.setProperty('--user-primary', accentColor);
    localStorage.setItem(accentStorageKey, accentColor);
  }, [accentColor, accentStorageKey]);

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
                  onClick={() => setTheme('light')}
                  aria-pressed={theme === 'light'}
                >
                  <Sun size={16} /> Light
                </button>
                <button
                  className={theme === 'dark' ? 'active' : ''}
                  onClick={() => setTheme('dark')}
                  aria-pressed={theme === 'dark'}
                >
                  <Moon size={16} /> Dark
                </button>
                <button
                  className={theme === 'system' ? 'active' : ''}
                  onClick={() => setTheme('system')}
                  aria-pressed={theme === 'system'}
                >
                  <Monitor size={16} /> Device
                </button>
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
            <div className="account-forms">
              <form className="account-form" onSubmit={(event) => void requestEmailChange(event)}>
                <div>
                  <h3>Change email</h3>
                  <p>We will send a confirmation link to your new address.</p>
                </div>
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
              <form className="account-form" onSubmit={(event) => void changePassword(event)}>
                <div>
                  <h3>Change password</h3>
                  <p>Use a password you do not use anywhere else.</p>
                </div>
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
            </div>
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
      </section>
    </>
  );
}
