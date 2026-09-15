import { useEffect, useState } from 'react';
import { Mail, Phone, Plus, Trash2, UserRound } from 'lucide-react';
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
          <p className="people-subtitle">Your contacts, available through CardDAV.</p>
        </div>
        <button className="button button-primary" onClick={() => edit()}>
          <Plus size={17} /> Add person
        </button>
      </header>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <div className="people-layout">
        <section className="people-list">
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
                onClick={() => edit(contact)}
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
