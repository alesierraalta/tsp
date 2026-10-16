export function findContact(contacts, query) {
  const key = (value) => value.normalize('NFC').toLowerCase();
  return contacts.find((contact) => key(contact.name) === key(query)) ?? null;
}

export function shortenLabel(label, limit) {
  if (!Number.isInteger(limit) || limit < 0) throw new RangeError('invalid limit');
  return Array.from(label).slice(0, limit).join('');
}
