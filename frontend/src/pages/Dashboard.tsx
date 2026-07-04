import { ServerList } from '../components/ServerList';

interface Props {
  onManage: (uuid: string) => void;
}

export function Dashboard({ onManage }: Props) {
  return (
    <div className="view active">
      <div className="dash-head">
        <h1>Servers</h1>
        <p>Everything you have access to, across every node.</p>
      </div>
      <ServerList onManage={onManage} />
    </div>
  );
}
