import Dialog from '../Dialog';
import type { RecallWorkspaceViewModel } from './types';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'actions'>;

export default function RecallActionDialog({ state, actions }: Props) {
  const pending = state.pendingAction;
  if (!pending) return null;
  return <Dialog
    title={pending.kind === 'move' ? '移动召回内容' : '删除召回内容'}
    description={pending.kind === 'move' ? '修改路径后会保留文件内容，并刷新当前打开的召回内容。' : '删除后会产生本地 Git 变更，需要同步后才会进入远端。'}
    onClose={() => { if (!state.busy) actions.closePendingAction(); }}
  >
    <div className="mem-lite-dialog-body">
      {pending.kind === 'move' ? (
        <label>
          <span>新的召回路径</span>
          <input value={pending.nextPath} onChange={(event) => actions.updatePendingMovePath(event.target.value)} />
        </label>
      ) : (
        <div className="mem-lite-danger-box">
          <strong>确认删除这条召回内容？</strong>
          <code>{pending.path}</code>
        </div>
      )}
      {'error' in pending && pending.error && <p className="mem-lite-dialog-error">{pending.error}</p>}
      <div className="mem-lite-dialog-actions">
        <button type="button" onClick={actions.closePendingAction} disabled={state.busy}>取消</button>
        {pending.kind === 'move'
          ? <button className="primary" type="button" onClick={actions.confirmMove} disabled={state.busy}>确认移动</button>
          : <button className="danger" type="button" onClick={actions.confirmDelete} disabled={state.busy}>确认删除</button>}
      </div>
    </div>
  </Dialog>;
}
