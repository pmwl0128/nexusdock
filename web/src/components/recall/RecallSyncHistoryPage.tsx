import { formatTime } from '../../lib/time';
import type { RecallWorkspaceViewModel } from './types';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'dirty' | 'actions'>;

export default function RecallSyncHistoryPage({ state, dirty, actions }: Props) {
  return <section className="recall-sync-page">
    <article className="recall-tool-panel recall-sync-panel">
      <div className="recall-panel-head"><div><h2>同步状态</h2><p>查看当前分支状态，并执行明确的拉取或保存动作。</p></div></div>
      <div className="recall-sync-state">
        <div><span>分支</span><strong>{String(state.syncStatus?.branch || '默认')}</strong></div>
        <div><span>领先</span><strong>{String(state.syncStatus?.ahead ?? '0')}</strong></div>
        <div><span>落后</span><strong>{String(state.syncStatus?.behind ?? '0')}</strong></div>
        <div><span>状态</span><strong>{dirty ? '需要同步' : '健康'}</strong></div>
      </div>
      <div className="recall-sync-actions">
        <button type="button" onClick={() => actions.syncNow('pull')} disabled={state.busy}>从远端更新</button>
        <button type="button" onClick={() => actions.syncNow('push')} disabled={state.busy}>保存到远端</button>
        <button type="button" className="primary" onClick={() => actions.syncNow('now')} disabled={state.busy}>更新并保存</button>
      </div>
    </article>

    <article className="recall-tool-panel recall-history-panel">
      <div className="recall-panel-head"><div><h2>同步历史</h2><p>展示最近版本时间线；详细 Diff 继续交给 Git 工具或 Agent。</p></div></div>
      <div className="recall-commits">
        {state.commits.length === 0 ? <p className="recall-empty">暂无版本记录。</p> : state.commits.map((commit) => (
          <div key={commit.hash}>
            <span />
            <div><strong>{commit.subject || '(无说明)'}</strong><small>{commit.short_hash} · {commit.author} · {formatTime(commit.date)}</small></div>
          </div>
        ))}
      </div>
    </article>
  </section>;
}
