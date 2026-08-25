import type { RecallPage, RecallWorkspaceViewModel } from './types';
import RecallActionDialog from './RecallActionDialog';
import RecallEditor from './RecallEditor';
import RecallExperienceCardsPage from './RecallExperienceCardsPage';
import RecallFileBrowser from './RecallFileBrowser';
import RecallHeader from './RecallHeader';
import RecallNoticeArea from './RecallNoticeArea';
import RecallStats from './RecallStats';
import RecallSyncHistoryPage from './RecallSyncHistoryPage';
import RecallVectorRecallPage from './RecallVectorRecallPage';

type Props = RecallWorkspaceViewModel & {
  page: RecallPage;
  onNavigate: (page: RecallPage) => void;
};

const recallNavigation: Array<{ id: RecallPage; label: string; description: string }> = [
  { id: 'library', label: '资料库', description: '浏览与编辑' },
  { id: 'cards', label: '经验卡片', description: '捕获与写入' },
  { id: 'vectors', label: '向量召回', description: '语义搜索' },
  { id: 'history', label: '同步历史', description: '状态与版本' },
];

export default function RecallWorkspaceView(props: Props) {
  function openCardFromTools(path: string) {
    props.onNavigate('library');
    props.actions.openSimilarCard(path);
  }

  return <main className={`recall-workspace ${props.detailOpen ? 'is-detail-open' : ''}`}>
    <RecallHeader {...props} />
    <nav className="recall-subnav" aria-label="Recall 分类">
      {recallNavigation.map((item) => <button type="button" key={item.id} className={props.page === item.id ? 'is-active' : ''} aria-current={props.page === item.id ? 'page' : undefined} onClick={() => props.onNavigate(item.id)}><strong>{item.label}</strong><span>{item.description}</span></button>)}
    </nav>
    <RecallNoticeArea {...props} />
    <RecallActionDialog {...props} />

    {props.page === 'library' && <>
      <RecallStats {...props} />
      <section className="recall-grid">
        <RecallFileBrowser {...props} />
        <RecallEditor {...props} />
      </section>
    </>}
    {props.page === 'cards' && <RecallExperienceCardsPage state={props.state} actions={props.actions} onOpenCard={openCardFromTools} />}
    {props.page === 'vectors' && <RecallVectorRecallPage state={props.state} actions={props.actions} onOpenCard={openCardFromTools} />}
    {props.page === 'history' && <RecallSyncHistoryPage state={props.state} dirty={props.dirty} actions={props.actions} />}
  </main>;
}
