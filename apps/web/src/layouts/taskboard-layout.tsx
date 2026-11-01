import { defineLayout } from '../lib/slopstack';
import { MasterDetailLayout } from '../components/patterns/application-layouts';

export const taskboardLayout = defineLayout({
  id: 'list-detail',
  label: 'List and entries',
  slots: [
    { id: 'master', label: 'Lists', accepts: ['component'], required: true },
    { id: 'detail', label: 'Selected list', accepts: ['component'], required: true },
  ],
  component: ({ slots }) => (
    <MasterDetailLayout
      master={slots.master}
      detail={slots.detail}
      masterLabel="Lists"
      detailLabel="Selected list"
    />
  ),
});
