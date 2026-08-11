import { useState } from 'react';
import { CreateServerForm } from '../components/CreateServerForm';
import { ServerList } from '../components/ServerList';
import { t } from '../i18n';

interface Props {
  onManage: (uuid: string) => void;
}

export function Dashboard({ onManage }: Props) {
  const [refreshKey, setRefreshKey] = useState(0);

  return (
    <div className="view active">
      <div className="dash-head">
        <h1>{t('dashboard.title')}</h1>
        <p>{t('dashboard.subtitle')}</p>
      </div>
      <CreateServerForm onCreated={() => setRefreshKey((k) => k + 1)} />
      <ServerList key={refreshKey} onManage={onManage} />
    </div>
  );
}
