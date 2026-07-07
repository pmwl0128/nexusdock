import { Check } from 'lucide-react';
import { clearRecallDraft } from '../../lib/drafts';
import type { RecallWorkspaceViewModel } from './types';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'actions'>;

export default function RecallNoticeArea({ state, actions }: Props) {
  return <>
    {state.notice && <div className={`mem-lite-notice ${state.notice.danger ? 'danger' : ''}`}>{state.notice.danger ? null : <Check size={16} />}{state.notice.text}</div>}
    {state.draftAvailable && !state.editing && <div className="mem-lite-notice"><span>检测到未提交草稿。</span><button type="button" onClick={actions.restoreDraft}>恢复草稿</button><button type="button" onClick={() => { clearRecallDraft(); actions.discardDraft(); }}>丢弃</button></div>}
  </>;
}
