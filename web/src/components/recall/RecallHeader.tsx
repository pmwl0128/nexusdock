import { RefreshCw, UploadCloud } from 'lucide-react';
import type { RecallWorkspaceViewModel } from './types';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'changedCount' | 'dirty' | 'actions'>;

export default function RecallHeader({ state, changedCount, dirty, actions }: Props) {
  return <header className="mem-lite-header">
    <div>
      <span className="mem-lite-kicker">NEXUS RECALL</span>
      <h1>召回库</h1>
    </div>
    <div className="mem-lite-header-actions">
      <span className={`mem-lite-health ${dirty ? 'warn' : 'ok'}`}>{dirty ? `${changedCount} 项待同步` : '已同步'}</span>
      <button type="button" onClick={actions.refreshAll} disabled={state.loading || state.busy}><RefreshCw size={15} /><span>刷新</span></button>
      <button type="button" className="primary" onClick={() => actions.syncNow()} disabled={state.busy}><UploadCloud size={15} /><span>立即同步</span></button>
    </div>
  </header>;
}
