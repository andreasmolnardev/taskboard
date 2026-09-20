import { useEffect, useState } from 'react';
import { Mail, Pencil, Phone, Plus, Share2, Trash2, UserRound } from 'lucide-react';
import { pb } from '../../api/pocketbase';

type Contact = {
  id: string;
  uid?: string;
  formatted_name: string;
  given_name?: string;
  family_name?: string;
  email?: string;
  phone?: string;
  organization?: string;
  title?: string;
};

type ContactForm = {
  formatted_name: string;
  given_name: string;
  family_name: string;
  email: string;
  phone: string;
  organization: string;
  title: string;
};

const emptyForm: ContactForm = {
  formatted_name: '',
  given_name: '',
  family_name: '',
  email: '',
  phone: '',
  organization: '',
  title: '',
};

export function PeopleTab() {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [books, setBooks] = useState<{ id: string; name: string }[]>([]);
  const [selected, setSelected] = useState<Contact | null>(null);
  const [form, setForm] = useState<ContactForm>(emptyForm);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [editorOpen, setEditorOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const user = pb.authStore.record?.id;

  const load = async () => {
    if (!user) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError('');
    try {
      const filter = `owner = "${user}"`;
      const addressBooks = await pb
        .collection('address_books')
        .getFullList<{ id: string; name: string }>({ filter, sort: 'name' });
      if (addressBooks.length === 0) {
        const book = await pb
          .collection('address_books')
          .create({ owner: user, name: 'Contacts', color: '#7657a8' });
        addressBooks.push({ id: book.id, name: book.name });
      }
      const nextContacts = await pb
        .collection('contacts')
        .getFullList<Contact>({ filter, sort: 'formatted_name' });
      setBooks(addressBooks);
      setContacts(nextContacts);
    } catch {
      setError('Could not load people.');
    } finally {
      setLoading(false);
    }
  };
  useEffect(() => {
    void load();
  }, [user]);

  const select = (contact: Contact) => {
    setSelected(contact);
    setEditorOpen(false);
    setError('');
  };
  const edit = (contact?: Contact) => {
    setSelected(contact ?? null);
    setEditorOpen(true);
    setForm(
      contact
        ? {
            formatted_name: contact.formatted_name,
            given_name: contact.given_name ?? '',
            family_name: contact.family_name ?? '',
            email: contact.email ?? '',
            phone: contact.phone ?? '',
            organization: contact.organization ?? '',
            title: contact.title ?? '',
          }
        : emptyForm,
    );
    setError('');
  };
  const clearEditor = () => {
    setSelected(null);
    setEditorOpen(false);
    setForm(emptyForm);
  };
  const save = async () => {
    if (!form.formatted_name.trim()) {
      setError('A name is required.');
      return;
    }
    if (!user || !books[0]) {
      setError('No address book is available.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const values = {
        ...form,
        owner: user,
        address_book: books[0].id,
        uid: selected?.uid ?? crypto.randomUUID(),
      };
      await (selected
        ? pb.collection('contacts').update(selected.id, values)
        : pb.collection('contacts').create(values));
      clearEditor();
      await load();
    } catch {
      setError('Could not save this person.');
    } finally {
      setBusy(false);
    }
  };
  const share = async () => {
    if (!selected) return;
    const details = [
      selected.formatted_name,
      selected.organization,
      selected.title,
      selected.email,
      selected.phone,
    ]
      .filter(Boolean)
      .join('\n');
    try {
      if (navigator.share) {
        await navigator.share({ title: selected.formatted_name, text: details });
      } else {
        await navigator.clipboard.writeText(details);
        setError('Contact details copied to the clipboard.');
      }
    } catch (error) {
      if (error instanceof Error && error.name !== 'AbortError') {
        setError('Could not share this person.');
      }
    }
  };
  const remove = async () => {
    if (!selected || busy) return;
    setBusy(true);
    setError('');
    try {
      await pb.collection('contacts').delete(selected.id);
      clearEditor();
      await load();
    } catch {
      setError('Could not delete this person.');
    } finally {
      setBusy(false);
    }
  };
  const updateForm = (key: keyof ContactForm, value: string) =>
    setForm((current) => ({ ...current, [key]: value }));

  return (
    <div className="people-page">
      <header className="page-header">
        <div>
          <h1>People</h1>
        </div>
      </header>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="people-layout">
        <section className="people-list">
          <button className="button button-primary people-add" onClick={() => edit()}>
            <Plus size={17} /> Add person
          </button>
          {loading ? (
            <p className="empty-state">Loading contacts…</p>
          ) : contacts.length === 0 ? (
            <div className="empty-state">
              <UserRound size={30} />
              <p>No contacts yet.</p>
            </div>
          ) : (
            contacts.map((contact) => (
              <button
                className={`person-row ${selected?.id === contact.id ? 'active' : ''}`}
                key={contact.id}
                onClick={() => select(contact)}
              >
                <span className="person-avatar">
                  <UserRound size={17} />
                </span>
                <span>
                  <strong>{contact.formatted_name}</strong>
                  <small>{contact.organization || contact.email || 'No details'}</small>
                </span>
              </button>
            ))
          )}
        </section>
        <section className="person-editor">
          {editorOpen ? (
            <>
              <div className="selected-list-header">
                <h2>{selected ? 'Edit person' : 'New person'}</h2>
                {selected && (
                  <button
                    className="icon-button"
                    onClick={() => void remove()}
                    aria-label="Delete contact"
                    disabled={busy}
                  >
                    <Trash2 size={18} />
                  </button>
                )}
              </div>
              <label>
                Name
                <input
                  value={form.formatted_name}
                  onChange={(event) => updateForm('formatted_name', event.target.value)}
                  placeholder="Full name"
                />
              </label>
              <div className="person-fields">
                <label>
                  First name
                  <input
                    value={form.given_name}
                    onChange={(event) => updateForm('given_name', event.target.value)}
                  />
                </label>
                <label>
                  Last name
                  <input
                    value={form.family_name}
                    onChange={(event) => updateForm('family_name', event.target.value)}
                  />
                </label>
                <label>
                  <Mail size={14} /> Email
                  <input
                    type="email"
                    value={form.email}
                    onChange={(event) => updateForm('email', event.target.value)}
                  />
                </label>
                <label>
                  <Phone size={14} /> Phone
                  <input
                    value={form.phone}
                    onChange={(event) => updateForm('phone', event.target.value)}
                  />
                </label>
                <label>
                  Organization
                  <input
                    value={form.organization}
                    onChange={(event) => updateForm('organization', event.target.value)}
                  />
                </label>
                <label>
                  Title
                  <input
                    value={form.title}
                    onChange={(event) => updateForm('title', event.target.value)}
                  />
                </label>
              </div>
              <button className="button button-primary" onClick={() => void save()} disabled={busy}>
                {busy ? 'Saving…' : 'Save person'}
              </button>
            </>
          ) : selected ? (
            <>
              <div className="selected-list-header">
                <h2>{selected.formatted_name}</h2>
                <div className="person-actions">
                  <button className="button button-secondary" onClick={() => edit(selected)}>
                    <Pencil size={16} /> Edit
                  </button>
                  <button className="icon-button" onClick={() => void share()} aria-label="Share contact">
                    <Share2 size={18} />
                  </button>
                  <button
                    className="icon-button"
                    onClick={() => void remove()}
                    aria-label="Delete contact"
                    disabled={busy}
                  >
                    <Trash2 size={18} />
                  </button>
                </div>
              </div>
              <div className="person-details">
                {selected.given_name && (
                  <p>
                    <strong>First name</strong>
                    {selected.given_name}
                  </p>
                )}
                {selected.family_name && (
                  <p>
                    <strong>Last name</strong>
                    {selected.family_name}
                  </p>
                )}
                {selected.email && (
                  <p>
                    <strong>Email</strong>
                    {selected.email}
                  </p>
                )}
                {selected.phone && (
                  <p>
                    <strong>Phone</strong>
                    {selected.phone}
                  </p>
                )}
                {selected.organization && (
                  <p>
                    <strong>Organization</strong>
                    {selected.organization}
                  </p>
                )}
                {selected.title && (
                  <p>
                    <strong>Title</strong>
                    {selected.title}
                  </p>
                )}
              </div>
            </>
          ) : (
            <div className="empty-state">
              <UserRound size={30} />
              <p>Select a person or add one.</p>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
