import type { RecallWorkspaceViewModel } from './types';
import RecallActionDialog from './RecallActionDialog';
import RecallCardsConsole from './RecallCardsConsole';
import RecallEditor from './RecallEditor';
import RecallFileBrowser from './RecallFileBrowser';
import RecallHeader from './RecallHeader';
import RecallNoticeArea from './RecallNoticeArea';
import RecallStats from './RecallStats';
import RecallSyncPanels from './RecallSyncPanels';

export default function RecallWorkspaceView(props: RecallWorkspaceViewModel) {
  return <main className={`mem-lite ${props.detailOpen ? 'is-detail-open' : ''}`}>
    <RecallHeader {...props} />
    <RecallNoticeArea {...props} />
    <RecallActionDialog {...props} />
    <RecallStats {...props} />
    <section className="mem-lite-grid">
      <RecallFileBrowser {...props} />
      <RecallEditor {...props} />
    </section>
    <details className="mem-lite-advanced">
      <summary><strong>高级工具</strong><span>经验卡片、向量召回、同步历史</span></summary>
      <RecallCardsConsole {...props} />
      <RecallSyncPanels {...props} />
    </details>
  </main>;
}
