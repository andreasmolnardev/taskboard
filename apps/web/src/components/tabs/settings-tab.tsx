import { useEffect, useState, type ChangeEvent } from 'react';
import { CalendarDays, Monitor, Moon, Palette, Sun, Trash2, Upload } from 'lucide-react';
import { useTheme } from 'next-themes';
import { pb } from '../../api/pocketbase';
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs';
import {
  accentColors,
  fontSizes,
  getWeekStart,
  weekStartStorageKey,
  type FontSize,
  type WeekStart,
} from '../../data';

type SettingsSection = 'appearance' | 'account' | 'calendar';
const settingsSections: SettingsSection[] = ['appearance', 'account', 'calendar'];

export function SettingsTab() {
  const { theme, setTheme } = useTheme();
  const userId = pb.authStore.record?.id ?? 'anonymous';
  const accentStorageKey = `taskboard-accent-${userId}`;
  const fontSizeStorageKey = `taskboard-font-size-${userId}`;
  const [accentColor, setAccentColor] = useState(
    () => localStorage.getItem(accentStorageKey) ?? accentColors[0].value,
  );
  const [fontSize, setFontSize] = useState<FontSize>(() => {
    const storedSize = localStorage.getItem(fontSizeStorageKey);
    return fontSizes.some((option) => option.value === storedSize) ? (storedSize as FontSize) : 'medium';
  });
  const [importedCalendars, setImportedCalendars] = useState<string[]>([]);
  const [weekStart, setWeekStart] = useState<WeekStart>(getWeekStart);
  const [appPasswords, setAppPasswords] = useState<{ id: string; name: string }[]>([]);
  const [newPasswordName, setNewPasswordName] = useState('CalDAV');
  const [newPassword, setNewPassword] = useState<string | null>(null);
  const [appPasswordBusy, setAppPasswordBusy] = useState(false);
  const [appPasswordError, setAppPasswordError] = useState('');
  const [activeSection, setActiveSection] = useState<SettingsSection>('appearance');
  const caldavBaseUrl = `${window.location.origin}/caldav`;
  const principalUrl = `${caldavBaseUrl}/principals/${encodeURIComponent(userId)}/`;
  const calendarHomeUrl = `${caldavBaseUrl}/calendars/${encodeURIComponent(userId)}/`;

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
    void fetch('/api/auth/app-passwords', {
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
      const response = await fetch('/api/auth/app-passwords', {
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
    const response = await fetch(`/api/auth/app-passwords/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${pb.authStore.token}` },
    });
    if (response.ok) setAppPasswords((current) => current.filter((password) => password.id !== id));
  };

  const handleImport = (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    setImportedCalendars((current) => [...current, ...files.map((file) => file.name)]);
    event.target.value = '';
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
          <div className="settings-section">
            <div>
              <h2>Account</h2>
              <p>{pb.authStore.record?.email}</p>
            </div>
            <button className="button button-quiet" onClick={() => pb.authStore.clear()}>
              Log out
            </button>
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
          <div>
            <h2>Calendars</h2>
            <p>Import an iCalendar file or connect a calendar through CalDAV.</p>
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
          <label className="button button-quiet import-button">
            <Upload size={16} /> Import calendar
            <input type="file" accept=".ics,.ical,text/calendar" multiple onChange={handleImport} />
          </label>
        </div>
        <div className="settings-section calendar-sync-settings">
          <div>
            <h2>Calendar sync</h2>
            <p>Use these URLs in your CalDAV client. Sign in with your account email and an app password.</p>
            <div className="sync-urls">
              <label>
                Server URL<code>{caldavBaseUrl}</code>
              </label>
              <label>
                Principal URL<code>{principalUrl}</code>
              </label>
              <label>
                Calendar home URL<code>{calendarHomeUrl}</code>
              </label>
            </div>
            <div className="app-passwords">
              <strong>CalDAV app passwords</strong>
              <p>App passwords can be used instead of your real password. You will only see the secret once.</p>
              <div className="app-password-create">
                <input
                  aria-label="App password name"
                  value={newPasswordName}
                  onChange={(event) => setNewPasswordName(event.target.value)}
                  placeholder="Name, for example iPhone"
                />
                <button className="button button-quiet" onClick={() => void createAppPassword()} disabled={appPasswordBusy}>
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
                  <button aria-label={`Revoke ${password.name || 'app password'}`} onClick={() => void revokeAppPassword(password.id)}>
                    <Trash2 size={14} />
                  </button>
                </div>
              ))}
            </div>
          </div>
          <span className="status-pill">
            <span /> Connected
          </span>
        </div>
          </>
        )}
      </section>
    </>
  );
}
